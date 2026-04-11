// Package main is the entry point for the aws-lambda-httpclient-deploy command-line tool.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"slices"
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
	"github.com/aws/smithy-go"
)

type lambda struct {
	functionName             string
	vpcID                    string
	subnetIDs                string
	roleInlinePolicyFile     string
	zipFile                  string
	architecture             string
	handler                  string
	runtime                  string
	logRetentionDays         int
	functionTimeoutInSeconds int
	memoryInMB               int
	destroy                  bool
	envFile                  string
}

func main() {
	var parameters lambda

	flag.StringVar(&parameters.functionName, "function-name", "aws-lambda-httpclient", "The name of the AWS Lambda function")
	flag.StringVar(&parameters.vpcID, "vpc-id", "", "The ID of the VPC to deploy the Lambda function in")
	flag.StringVar(&parameters.subnetIDs, "subnet-ids", "", "Space-separated IDs of the subnets to deploy the Lambda function in")
	flag.StringVar(&parameters.roleInlinePolicyFile, "role-inline-policy-file", "samples/lambda-role-policy.json", "Path to a JSON file containing the inline policy for the Lambda execution role")
	flag.StringVar(&parameters.zipFile, "zip-file", "lambda.zip", "Path to the ZIP file containing the Lambda function code")
	flag.StringVar(&parameters.architecture, "architecture", "x86_64", "Architecture for the Lambda function (x86_64 or arm64)")
	flag.StringVar(&parameters.handler, "handler", "main", "Handler for the Lambda function")
	flag.StringVar(&parameters.runtime, "runtime", "provided.al2023", "Runtime for the Lambda function")
	flag.IntVar(&parameters.logRetentionDays, "log-retention-days", 7, "Number of days to retain logs in CloudWatch")
	flag.IntVar(&parameters.functionTimeoutInSeconds, "function-timeout", 10, "Timeout for the Lambda function in seconds")
	flag.IntVar(&parameters.memoryInMB, "memory", 128, "Memory size for the Lambda function in MB")
	flag.StringVar(&parameters.envFile, "env-file", "samples/env.json", "Path to JSON file containing Lambda environment variables")
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

	envVars, errEnvFile := loadEnvVars(parameters.envFile)
	if errEnvFile != nil {
		log.Fatalf("load env file: %s: %v", parameters.envFile, errEnvFile)
	}

	errLambda := ensureLambda(ctx, lambdaClient, parameters.functionName,
		roleARN, zipBytes, subnetIDs, securityGroupID,
		int32(parameters.functionTimeoutInSeconds), int32(parameters.memoryInMB),
		parameters.architecture, parameters.handler, parameters.runtime, envVars)
	if errLambda != nil {
		log.Fatalf("ensure lambda: %v", errLambda)
	}

	// ensure cloudwatch log group

	errLogGroup := ensureLogGroup(ctx, logsClient, parameters.functionName,
		parameters.logRetentionDays)
	if errLogGroup != nil {
		log.Fatalf("ensure cloudwatch log group: %v", errLogGroup)
	}

	log.Printf("deployment complete: function=%s role=%s security_group=%s handler=%s",
		parameters.functionName, roleName, securityGroupID, parameters.handler)
}

func loadEnvVars(path string) (map[string]string, error) {
	data, errRead := os.ReadFile(path)
	if errRead != nil {
		return nil, errRead
	}

	var envVars map[string]string
	if errUnmarshal := json.Unmarshal(data, &envVars); errUnmarshal != nil {
		return nil, errUnmarshal
	}

	if envVars == nil {
		envVars = map[string]string{}
	}

	return envVars, nil
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
	securityGroupID string, functionTimeoutInSeconds,
	memoryInMB int32, architecture, handler, runtime string,
	envVars map[string]string) error {

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
			_, errCreate := client.CreateFunction(ctx,
				&lambdasvc.CreateFunctionInput{
					Architectures: []lambdatypes.Architecture{lambdatypes.Architecture(architecture)},
					Code:          &lambdatypes.FunctionCode{ZipFile: zipBytes},
					FunctionName:  aws.String(functionName),
					Description:   aws.String(functionName),
					Handler:       aws.String(handler),
					Role:          aws.String(roleARN),
					Runtime:       lambdatypes.Runtime(runtime),
					VpcConfig:     vpcConfig,
					Timeout:       aws.Int32(functionTimeoutInSeconds),
					MemorySize:    aws.Int32(memoryInMB),
					Environment:   &lambdatypes.Environment{Variables: envVars},
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
			Description:  aws.String(functionName),
			Handler:      aws.String(handler),
			Role:         aws.String(roleARN),
			Runtime:      lambdatypes.Runtime(runtime),
			VpcConfig:    vpcConfig,
			Timeout:      aws.Int32(functionTimeoutInSeconds),
			MemorySize:   aws.Int32(memoryInMB),
			Environment:  &lambdatypes.Environment{Variables: envVars},
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

	// initialize sdk aws client

	ctx := context.Background()

	cfg, errCfg := config.LoadDefaultConfig(ctx)
	if errCfg != nil {
		log.Fatalf("load aws config: %v", errCfg)
	}

	ec2Client := ec2.NewFromConfig(cfg)
	iamClient := iam.NewFromConfig(cfg)
	lambdaClient := lambdasvc.NewFromConfig(cfg)
	logsClient := cloudwatchlogs.NewFromConfig(cfg)

	// delete lambda function

	var errs []error

	if err := deleteLambdaFunction(ctx, lambdaClient, parameters.functionName); err != nil {
		errs = append(errs, err)
	}

	// delete role

	if err := deleteRole(ctx, iamClient, parameters.functionName); err != nil {
		errs = append(errs, err)
	}

	// delete security group if vpc id was provided

	if parameters.vpcID != "" {
		if err := deleteSecurityGroup(ctx, ec2Client, parameters.vpcID, parameters.functionName); err != nil {
			errs = append(errs, err)
		}
	} else {
		log.Printf("skipping security group deletion because -vpc-id was not provided")
	}

	// delete cloudwatch log group

	if err := deleteLogGroup(ctx, logsClient, parameters.functionName); err != nil {
		errs = append(errs, err)
	}

	// report errors if any

	if len(errs) > 0 {
		for _, err := range errs {
			log.Printf("destroy error: %v", err)
		}
		log.Printf("destroy finished with %d error(s)", len(errs))
		return
	}

	log.Printf("destroy complete: function=%s", parameters.functionName)
}

func deleteLambdaFunction(ctx context.Context, client *lambdasvc.Client, functionName string) error {
	log.Printf("deleting lambda function: name=%s", functionName)

	_, err := client.DeleteFunction(ctx, &lambdasvc.DeleteFunctionInput{
		FunctionName: aws.String(functionName),
	})
	if err != nil {
		var notFound *lambdatypes.ResourceNotFoundException
		if errors.As(err, &notFound) {
			log.Printf("lambda function not found: name=%s", functionName)
			return nil
		}
		return fmt.Errorf("delete lambda function %s: %w", functionName, err)
	}

	return nil
}

func deleteRole(ctx context.Context, client *iam.Client, roleName string) error {
	log.Printf("deleting role: name=%s", roleName)

	inlinePolicyName := roleName
	_, errDeleteInline := client.DeleteRolePolicy(ctx, &iam.DeleteRolePolicyInput{
		RoleName:   aws.String(roleName),
		PolicyName: aws.String(inlinePolicyName),
	})
	if errDeleteInline != nil {
		var notFound *iamtypes.NoSuchEntityException
		if !errors.As(errDeleteInline, &notFound) {
			return fmt.Errorf("delete inline policy %s from role %s: %w", inlinePolicyName, roleName, errDeleteInline)
		}
	}

	managedPolicies := []string{
		"arn:aws:iam::aws:policy/service-role/AWSLambdaBasicExecutionRole",
		"arn:aws:iam::aws:policy/service-role/AWSLambdaVPCAccessExecutionRole",
	}

	for _, policyARN := range managedPolicies {
		_, errDetach := client.DetachRolePolicy(ctx, &iam.DetachRolePolicyInput{
			RoleName:  aws.String(roleName),
			PolicyArn: aws.String(policyARN),
		})
		if errDetach != nil {
			var notFound *iamtypes.NoSuchEntityException
			if !errors.As(errDetach, &notFound) {
				return fmt.Errorf("detach policy %s from role %s: %w", policyARN, roleName, errDetach)
			}
		}
	}

	_, errDeleteRole := client.DeleteRole(ctx, &iam.DeleteRoleInput{RoleName: aws.String(roleName)})
	if errDeleteRole != nil {
		var notFound *iamtypes.NoSuchEntityException
		if errors.As(errDeleteRole, &notFound) {
			log.Printf("role not found: name=%s", roleName)
			return nil
		}
		return fmt.Errorf("delete role %s: %w", roleName, errDeleteRole)
	}

	return nil
}

func deleteSecurityGroup(ctx context.Context, client *ec2.Client, vpcID, groupName string) error {
	log.Printf("deleting security group: vpc=%s name=%s", vpcID, groupName)

	groupID, errFind := findSecurityGroupID(ctx, client, vpcID, groupName)
	if errFind != nil {
		return fmt.Errorf("find security group for deletion: %w", errFind)
	}
	if groupID == "" {
		log.Printf("security group not found: vpc=%s name=%s", vpcID, groupName)
		return nil
	}

	_, errDelete := client.DeleteSecurityGroup(ctx, &ec2.DeleteSecurityGroupInput{GroupId: aws.String(groupID)})
	if errDelete != nil {
		if isAWSErrorCode(errDelete, "InvalidGroup.NotFound") {
			log.Printf("security group already deleted: id=%s", groupID)
			return nil
		}
		return fmt.Errorf("delete security group %s: %w", groupID, errDelete)
	}

	return nil
}

func deleteLogGroup(ctx context.Context, client *cloudwatchlogs.Client, functionName string) error {
	log.Printf("deleting log group: function=%s", functionName)

	logGroupName := "/aws/lambda/" + functionName
	_, err := client.DeleteLogGroup(ctx, &cloudwatchlogs.DeleteLogGroupInput{LogGroupName: aws.String(logGroupName)})
	if err != nil {
		var notFound *cloudwatchlogstypes.ResourceNotFoundException
		if errors.As(err, &notFound) {
			log.Printf("log group not found: name=%s", logGroupName)
			return nil
		}
		return fmt.Errorf("delete log group %s: %w", logGroupName, err)
	}

	return nil
}

func isAWSErrorCode(err error, codes ...string) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	return slices.Contains(codes, apiErr.ErrorCode())
}
