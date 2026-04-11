// Package main is the entry point for the aws-lambda-httpclient-deploy command-line tool.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cloudwatchlogstypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	lambdasvc "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
)

type lambda struct {
	functionName         string
	vpcID                string
	subnetIDs            string
	roleInlinePolicyFile string
	zipFile              string
	logRetentionDays     int
	destroy              bool
}

func main() {
	var parameters lambda

	flag.StringVar(&parameters.functionName, "function-name", "aws-lambda-httpclient", "The name of the AWS Lambda function")
	flag.StringVar(&parameters.vpcID, "vpc-id", "", "The ID of the VPC to deploy the Lambda function in")
	flag.StringVar(&parameters.subnetIDs, "subnet-ids", "", "Space-separated IDs of the subnets to deploy the Lambda function in")
	flag.StringVar(&parameters.roleInlinePolicyFile, "role-inline-policy-file", "samples/lambda-role-policy.json", "Path to a JSON file containing the inline policy for the Lambda execution role")
	flag.StringVar(&parameters.zipFile, "zip-file", "lambda.zip", "Path to the ZIP file containing the Lambda function code")
	flag.IntVar(&parameters.logRetentionDays, "log-retention-days", 7, "Number of days to retain logs in CloudWatch")
	flag.BoolVar(&parameters.destroy, "destroy", false, "Whether to destroy the deployed Lambda function")

	flag.Parse()

	if parameters.destroy {
		destroyLambda(parameters)
	} else {
		deployLambda(parameters)
	}
}

// deployLambda creates or updates the AWS Lambda function and its associated resources based on the provided parameters.
func deployLambda(parameters lambda) {
	subnetIDs := strings.Fields(parameters.subnetIDs)

	//
	// initialize sdk aws client
	//

	ctx := context.Background()

	cfg, errCfg := config.LoadDefaultConfig(ctx)
	if errCfg != nil {
		log.Fatalf("load aws config: %v", errCfg)
	}

	ec2Client := ec2.NewFromConfig(cfg)
	iamClient := iam.NewFromConfig(cfg)
	lambdaClient := lambdasvc.NewFromConfig(cfg)
	logsClient := cloudwatchlogs.NewFromConfig(cfg)

	// ensure security group

	var securityGroupID string

	if parameters.vpcID != "" {
		securityGroupName := parameters.functionName
		sgID, errSG := ensureSecurityGroup(ctx, ec2Client,
			parameters.vpcID, securityGroupName)
		if errSG != nil {
			log.Fatalf("ensure security group: %v", errSG)
		}
		securityGroupID = sgID
	}

	// load role inline policy

	inlinePolicyBytes, errPolicyFile := os.ReadFile(parameters.roleInlinePolicyFile)
	if errPolicyFile != nil {
		log.Fatalf("read role inline policy file: %s: %v",
			parameters.roleInlinePolicyFile, errPolicyFile)
	}

	// ensure role

	roleName := parameters.functionName
	roleARN, errRole := ensureRole(ctx, iamClient, roleName,
		string(inlinePolicyBytes))
	if errRole != nil {
		log.Fatalf("ensure role: %v", errRole)
	}

	// load lambda zip file

	zipBytes, errZip := os.ReadFile(parameters.zipFile)
	if errZip != nil {
		log.Fatalf("read lambda zip file: %s: %v", parameters.zipFile, errZip)
	}

	// ensure lambda function

	errLambda := ensureLambda(ctx, lambdaClient, parameters.functionName,
		roleARN, zipBytes, subnetIDs, securityGroupID)
	if errLambda != nil {
		log.Fatalf("ensure lambda: %v", errLambda)
	}

	// ensure cloudwatch log group

	errLogGroup := ensureLogGroup(ctx, logsClient, parameters.functionName,
		parameters.logRetentionDays)
	if errLogGroup != nil {
		log.Fatalf("ensure cloudwatch log group: %v", errLogGroup)
	}

	log.Printf("deployment complete: function=%s role=%s security_group=%s",
		parameters.functionName, roleName, securityGroupID)
}

func ensureSecurityGroup(ctx context.Context, client *ec2.Client, vpcID,
	groupName string) (string, error) {

	log.Printf("ensuring security group: vpc=%s name=%s", vpcID, groupName)

	groupID, errFind := findSecurityGroupID(ctx, client, vpcID, groupName)
	if errFind != nil {
		return "", fmt.Errorf("find security group: %w", errFind)
	}

	if groupID == "" {
		createOut, errCreate := client.CreateSecurityGroup(ctx,
			&ec2.CreateSecurityGroupInput{
				Description: aws.String("security group for lambda " + groupName),
				GroupName:   aws.String(groupName),
				VpcId:       aws.String(vpcID),
			})
		if errCreate != nil {
			return "", fmt.Errorf("create security group: %w", errCreate)
		}
		groupID = aws.ToString(createOut.GroupId)
	}

	describeOut, errDescribe := client.DescribeSecurityGroups(ctx,
		&ec2.DescribeSecurityGroupsInput{
			GroupIds: []string{groupID},
		})
	if errDescribe != nil {
		return "", fmt.Errorf("describe security group %s: %w", groupID,
			errDescribe)
	}

	if len(describeOut.SecurityGroups) != 1 {
		return "", fmt.Errorf("security group %s not found after create/update",
			groupID)
	}

	sg := describeOut.SecurityGroups[0]

	if len(sg.IpPermissions) > 0 {
		_, errRevokeIngress := client.RevokeSecurityGroupIngress(ctx,
			&ec2.RevokeSecurityGroupIngressInput{
				GroupId:       aws.String(groupID),
				IpPermissions: sg.IpPermissions,
			})
		if errRevokeIngress != nil {
			return "", fmt.Errorf("revoke ingress rules from security group %s: %w",
				groupID, errRevokeIngress)
		}
	}

	if len(sg.IpPermissionsEgress) > 0 {
		_, errRevokeEgress := client.RevokeSecurityGroupEgress(ctx,
			&ec2.RevokeSecurityGroupEgressInput{
				GroupId:       aws.String(groupID),
				IpPermissions: sg.IpPermissionsEgress,
			})
		if errRevokeEgress != nil {
			return "", fmt.Errorf("revoke egress rules from security group %s: %w",
				groupID, errRevokeEgress)
		}
	}

	egressRules := []ec2types.IpPermission{
		{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int32(80),
			ToPort:     aws.Int32(80),
			IpRanges: []ec2types.IpRange{
				{CidrIp: aws.String("0.0.0.0/0")},
			},
			Ipv6Ranges: []ec2types.Ipv6Range{
				{CidrIpv6: aws.String("::/0")},
			},
		},
		{
			IpProtocol: aws.String("tcp"),
			FromPort:   aws.Int32(443),
			ToPort:     aws.Int32(443),
			IpRanges: []ec2types.IpRange{
				{CidrIp: aws.String("0.0.0.0/0")},
			},
			Ipv6Ranges: []ec2types.Ipv6Range{
				{CidrIpv6: aws.String("::/0")},
			},
		},
	}

	_, errAuthorize := client.AuthorizeSecurityGroupEgress(ctx,
		&ec2.AuthorizeSecurityGroupEgressInput{
			GroupId:       aws.String(groupID),
			IpPermissions: egressRules,
		})
	if errAuthorize != nil {
		return "", fmt.Errorf("authorize egress rules for security group %s: %w",
			groupID, errAuthorize)
	}

	return groupID, nil
}

func findSecurityGroupID(ctx context.Context, client *ec2.Client, vpcID,
	groupName string) (string, error) {

	log.Printf("finding security group: vpc=%s name=%s", vpcID, groupName)

	out, errDescribe := client.DescribeSecurityGroups(ctx,
		&ec2.DescribeSecurityGroupsInput{
			Filters: []ec2types.Filter{
				{Name: aws.String("vpc-id"), Values: []string{vpcID}},
				{Name: aws.String("group-name"), Values: []string{groupName}},
			},
		})
	if errDescribe != nil {
		return "", errDescribe
	}

	if len(out.SecurityGroups) == 0 {
		return "", nil
	}

	return aws.ToString(out.SecurityGroups[0].GroupId), nil
}

func ensureRole(ctx context.Context, client *iam.Client, roleName,
	inlinePolicy string) (string, error) {

	log.Printf("ensuring role: name=%s", roleName)

	roleARN, errFind := findRoleARN(ctx, client, roleName)
	if errFind != nil {
		return "", fmt.Errorf("find role: %w", errFind)
	}

	if roleARN == "" {
		trustPolicy := `{
	"Version":"2012-10-17",
	"Statement":[
		{
			"Effect":"Allow",
			"Principal":{"Service":"lambda.amazonaws.com"},
			"Action":"sts:AssumeRole"
		}
	]
}`

		createOut, errCreate := client.CreateRole(ctx, &iam.CreateRoleInput{
			AssumeRolePolicyDocument: aws.String(trustPolicy),
			RoleName:                 aws.String(roleName),
		})
		if errCreate != nil {
			return "", fmt.Errorf("create role: %w", errCreate)
		}
		roleARN = aws.ToString(createOut.Role.Arn)
	}

	inlinePolicyName := roleName
	_, errInline := client.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       aws.String(roleName),
		PolicyName:     aws.String(inlinePolicyName),
		PolicyDocument: aws.String(inlinePolicy),
	})
	if errInline != nil {
		return "", fmt.Errorf("put inline policy %s: %w", inlinePolicyName,
			errInline)
	}

	requiredManagedPolicies := []string{
		"arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole",
		"arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole",
	}

	for _, managedPolicyARN := range requiredManagedPolicies {
		_, errAttach := client.AttachRolePolicy(ctx, &iam.AttachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: aws.String(managedPolicyARN),
		})
		if errAttach != nil {
			return "", fmt.Errorf("attach managed policy %s: %w",
				managedPolicyARN, errAttach)
		}
	}

	return roleARN, nil
}

func findRoleARN(ctx context.Context, client *iam.Client,
	roleName string) (string, error) {

	log.Printf("finding role: name=%s", roleName)

	out, errGet := client.GetRole(ctx,
		&iam.GetRoleInput{RoleName: aws.String(roleName)})
	if errGet != nil {
		var notFound *iamtypes.NoSuchEntityException
		if errors.As(errGet, &notFound) {
			return "", nil
		}
		return "", errGet
	}

	if out.Role == nil {
		return "", nil
	}

	return aws.ToString(out.Role.Arn), nil
}

func ensureLambda(ctx context.Context, client *lambdasvc.Client, functionName,
	roleARN string, zipBytes []byte, subnetIDs []string,
	securityGroupID string) error {

	log.Printf("ensuring lambda function: name=%s", functionName)

	var vpcConfig *lambdatypes.VpcConfig

	if len(subnetIDs) > 0 {
		vpcConfig = &lambdatypes.VpcConfig{
			SubnetIds: subnetIDs,
		}
	}

	if securityGroupID != "" {
		if vpcConfig == nil {
			vpcConfig = &lambdatypes.VpcConfig{}
		}
		vpcConfig.SecurityGroupIds = []string{securityGroupID}
	}

	_, errGet := client.GetFunction(ctx,
		&lambdasvc.GetFunctionInput{FunctionName: aws.String(functionName)})
	if errGet != nil {
		var notFound *lambdatypes.ResourceNotFoundException
		if errors.As(errGet, &notFound) {
			_, errCreate := client.CreateFunction(ctx, &lambdasvc.CreateFunctionInput{
				Architectures: []lambdatypes.Architecture{lambdatypes.ArchitectureArm64},
				Code:          &lambdatypes.FunctionCode{ZipFile: zipBytes},
				FunctionName:  aws.String(functionName),
				Handler:       aws.String("bootstrap"),
				Role:          aws.String(roleARN),
				Runtime:       lambdatypes.RuntimeProvidedal2023,
				VpcConfig:     vpcConfig,
			})
			if errCreate != nil {
				return fmt.Errorf("create function: %w", errCreate)
			}
			return nil
		}
		return fmt.Errorf("get function: %w", errGet)
	}

	_, errCode := client.UpdateFunctionCode(ctx, &lambdasvc.UpdateFunctionCodeInput{
		FunctionName: aws.String(functionName),
		ZipFile:      zipBytes,
	})
	if errCode != nil {
		return fmt.Errorf("update function code: %w", errCode)
	}

	_, errCfg := client.UpdateFunctionConfiguration(ctx,
		&lambdasvc.UpdateFunctionConfigurationInput{
			FunctionName: aws.String(functionName),
			Handler:      aws.String("bootstrap"),
			Role:         aws.String(roleARN),
			Runtime:      lambdatypes.RuntimeProvidedal2023,
			VpcConfig:    vpcConfig,
		})
	if errCfg != nil {
		return fmt.Errorf("update function configuration: %w", errCfg)
	}

	return nil
}

func ensureLogGroup(ctx context.Context, client *cloudwatchlogs.Client,
	functionName string, retentionDays int) error {

	log.Printf("ensuring log group: name=%s", functionName)

	if retentionDays <= 0 {
		return fmt.Errorf("invalid log retention days: %d", retentionDays)
	}

	logGroupName := "/aws/lambda/" + functionName
	_, errCreate := client.CreateLogGroup(ctx,
		&cloudwatchlogs.CreateLogGroupInput{LogGroupName: aws.String(logGroupName)})
	if errCreate != nil {
		var alreadyExists *cloudwatchlogstypes.ResourceAlreadyExistsException
		if !errors.As(errCreate, &alreadyExists) {
			return fmt.Errorf("create log group %s: %w", logGroupName, errCreate)
		}
	}

	_, errRetention := client.PutRetentionPolicy(ctx,
		&cloudwatchlogs.PutRetentionPolicyInput{
			LogGroupName:    aws.String(logGroupName),
			RetentionInDays: aws.Int32(int32(retentionDays)),
		})
	if errRetention != nil {
		return fmt.Errorf("set retention policy for log group %s: %w",
			logGroupName, errRetention)
	}

	return nil
}

// destroyLambda deletes the AWS Lambda function and its associated resources based on the provided parameters.
func destroyLambda(parameters lambda) {
	// attempt to delete everything but do not stop midway if any of the delete operations fail.

	// destroy the lambda function.
	// destroy the role.
	// destroy the security group.
	// destroy the cloudwatch log group for the lambda function.
}
