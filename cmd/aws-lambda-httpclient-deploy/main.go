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
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs"
	cloudwatchlogstypes "github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs/types"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	kmstypes "github.com/aws/aws-sdk-go-v2/service/kms/types"
	lambdasvc "github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/smithy-go"
	"gopkg.in/yaml.v3"
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
	envFile                  string
	lambdaRoleArn            string
	sgEgressEntries          string
	logRetentionDays         int
	functionTimeoutInSeconds int
	memoryInMB               int
	createKmsKey             bool
	destroy                  bool
}

const programVersion = "0.0.1"

const defaultSgEgressEntries = `[{"proto":"tcp","FromPort":80,"ToPort":80,"Ip":["0.0.0.0/0"],"Ipv6":["::/0"]},{"proto":"tcp","FromPort":443,"ToPort":443,"Ip":["0.0.0.0/0"],"Ipv6":["::/0"]}]`

func main() {
	const me = "aws-lambda-httpclient-deploy"

	fmt.Printf("%s version %s\n", me, programVersion)

	var parameters lambda
	var showVersion bool

	flag.StringVar(&parameters.functionName, "function-name", "aws-lambda-httpclient", "The name of the AWS Lambda function")
	flag.StringVar(&parameters.vpcID, "vpc-id", "", "The ID of the VPC to deploy the Lambda function in")
	flag.StringVar(&parameters.subnetIDs, "subnet-ids", "", "Space-separated IDs of the subnets to deploy the Lambda function in")
	flag.StringVar(&parameters.roleInlinePolicyFile, "role-inline-policy-file", "samples/lambda-role-policy.json", "Path to a JSON file containing the inline policy for the Lambda execution role")
	flag.StringVar(&parameters.zipFile, "zip-file", "lambda.zip", "Path to the ZIP file containing the Lambda function code")
	flag.StringVar(&parameters.architecture, "architecture", "x86_64", "Architecture for the Lambda function (x86_64 or arm64)")
	flag.StringVar(&parameters.handler, "handler", "main", "Handler for the Lambda function")
	flag.StringVar(&parameters.runtime, "runtime", "provided.al2023", "Runtime for the Lambda function")
	flag.StringVar(&parameters.envFile, "env-file", "samples/env.yaml", "Path to YAML file containing Lambda environment variables")
	flag.StringVar(&parameters.lambdaRoleArn, "lambda-role-arn", "", "ARN of an existing IAM role to use for the Lambda function (if not provided, a new role will be created)")
	flag.StringVar(&parameters.sgEgressEntries, "sg-egresss", defaultSgEgressEntries, "JSON egress rules for the Lambda function security group")
	flag.IntVar(&parameters.logRetentionDays, "log-retention-days", 7, "Number of days to retain logs in CloudWatch")
	flag.IntVar(&parameters.functionTimeoutInSeconds, "function-timeout", 10, "Timeout for the Lambda function in seconds")
	flag.IntVar(&parameters.memoryInMB, "memory", 128, "Memory size for the Lambda function in MB")
	flag.BoolVar(&parameters.createKmsKey, "create-kms-key", false, "Whether to create a KMS key for the Lambda function")
	flag.BoolVar(&parameters.destroy, "destroy", false, "Whether to destroy the deployed Lambda function")
	flag.BoolVar(&showVersion, "version", false, "Show program version")

	flag.Parse()

	if showVersion {
		return
	}

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
	kmsClient := kms.NewFromConfig(cfg)
	lambdaClient := lambdasvc.NewFromConfig(cfg)
	logsClient := cloudwatchlogs.NewFromConfig(cfg)

	// ensure security group

	var securityGroupID string

	if parameters.vpcID != "" {
		securityGroupName := parameters.functionName
		sgID, errSG := ensureSecurityGroup(ctx, ec2Client,
			parameters.vpcID, securityGroupName, parameters.sgEgressEntries)
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
	var roleARN string
	var errRole error
	if parameters.lambdaRoleArn != "" {
		// role ARN provided, skip role creation.
		roleARN = parameters.lambdaRoleArn
	} else {
		// no role ARN provided, ensure role exists or create new one.
		roleARN, errRole = ensureRole(ctx, iamClient, roleName,
			string(inlinePolicyBytes))
		if errRole != nil {
			log.Fatalf("ensure role: %v", errRole)
		}
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

	var kmsKeyARN string

	if parameters.createKmsKey {
		key, errKMS := ensureKMSKey(ctx, kmsClient, parameters.functionName,
			roleARN)
		if errKMS != nil {
			log.Fatalf("ensure kms key: %v", errKMS)
		}
		kmsKeyARN = key
	}

	errLambda := ensureLambda(ctx, lambdaClient, parameters.functionName,
		roleARN, zipBytes, subnetIDs, securityGroupID,
		int32(parameters.functionTimeoutInSeconds), int32(parameters.memoryInMB),
		parameters.architecture, parameters.handler, parameters.runtime,
		envVars, kmsKeyARN)
	if errLambda != nil {
		log.Fatalf("ensure lambda: %v", errLambda)
	}

	// ensure cloudwatch log group

	errLogGroup := ensureLogGroup(ctx, logsClient, parameters.functionName,
		parameters.logRetentionDays)
	if errLogGroup != nil {
		log.Fatalf("ensure cloudwatch log group: %v", errLogGroup)
	}

	log.Printf("deployment complete: function=%s role=%s security_group=%s handler=%s kms_key=%s",
		parameters.functionName, roleName, securityGroupID, parameters.handler, kmsKeyARN)
}

func loadEnvVars(path string) (map[string]string, error) {
	data, errRead := os.ReadFile(path)
	if errRead != nil {
		return nil, errRead
	}

	var envVars map[string]string
	if errUnmarshal := yaml.Unmarshal(data, &envVars); errUnmarshal != nil {
		return nil, errUnmarshal
	}

	if envVars == nil {
		envVars = map[string]string{}
	}

	return envVars, nil
}

func ensureSecurityGroup(ctx context.Context, client *ec2.Client, vpcID,
	groupName, sgEgressEntries string) (string, error) {

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

	egressRules, errParse := parseSgEgressEntries(sgEgressEntries)
	if errParse != nil {
		return "", fmt.Errorf("parse security group egress entries: %w", errParse)
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

type sgEgressEntry struct {
	Proto    string   `json:"proto"`
	FromPort *int32   `json:"FromPort"`
	ToPort   *int32   `json:"ToPort"`
	IP       []string `json:"Ip"`
	IPv6     []string `json:"Ipv6"`
}

func parseSgEgressEntries(raw string) ([]ec2types.IpPermission, error) {
	var entries []sgEgressEntry
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, err
	}

	ipPermissions := make([]ec2types.IpPermission, 0, len(entries))
	for _, e := range entries {
		perm := ec2types.IpPermission{
			IpProtocol: aws.String(e.Proto),
			FromPort:   e.FromPort,
			ToPort:     e.ToPort,
		}

		if len(e.IP) > 0 {
			perm.IpRanges = make([]ec2types.IpRange, 0, len(e.IP))
			for _, cidr := range e.IP {
				perm.IpRanges = append(perm.IpRanges, ec2types.IpRange{CidrIp: aws.String(cidr)})
			}
		}

		if len(e.IPv6) > 0 {
			perm.Ipv6Ranges = make([]ec2types.Ipv6Range, 0, len(e.IPv6))
			for _, cidr := range e.IPv6 {
				perm.Ipv6Ranges = append(perm.Ipv6Ranges, ec2types.Ipv6Range{CidrIpv6: aws.String(cidr)})
			}
		}

		ipPermissions = append(ipPermissions, perm)
	}

	return ipPermissions, nil
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

func kmsAliasName(functionName string) string {
	return "alias/" + functionName
}

func ensureKMSKey(ctx context.Context, client *kms.Client, functionName,
	roleARN string) (string, error) {

	alias := kmsAliasName(functionName)
	log.Printf("ensuring kms key: alias=%s", alias)

	keyID, keyARN, errFind := findKMSKeyByAlias(ctx, client, alias)
	if errFind != nil {
		return "", fmt.Errorf("find kms key by alias %s: %w", alias, errFind)
	}

	if keyID == "" {
		createOut, errCreate := client.CreateKey(ctx, &kms.CreateKeyInput{
			Description: aws.String("KMS key for Lambda environment variables: " + functionName),
			KeySpec:     kmstypes.KeySpecSymmetricDefault,
			KeyUsage:    kmstypes.KeyUsageTypeEncryptDecrypt,
			Origin:      kmstypes.OriginTypeAwsKms,
		})
		if errCreate != nil {
			return "", fmt.Errorf("create kms key: %w", errCreate)
		}

		if createOut.KeyMetadata == nil {
			return "", fmt.Errorf("create kms key returned empty metadata")
		}

		keyID = aws.ToString(createOut.KeyMetadata.KeyId)
		keyARN = aws.ToString(createOut.KeyMetadata.Arn)

		_, errAlias := client.CreateAlias(ctx, &kms.CreateAliasInput{
			AliasName:   aws.String(alias),
			TargetKeyId: aws.String(keyID),
		})
		if errAlias != nil {
			if !isAWSErrorCode(errAlias, "AlreadyExistsException") {
				return "", fmt.Errorf("create kms alias %s: %w", alias, errAlias)
			}

			keyIDExisting, keyARNExisting, errExisting := findKMSKeyByAlias(ctx, client, alias)
			if errExisting != nil {
				return "", fmt.Errorf("find existing kms alias %s after conflict: %w", alias, errExisting)
			}
			if keyIDExisting == "" {
				return "", fmt.Errorf("kms alias %s exists but target key was not found", alias)
			}
			keyID = keyIDExisting
			keyARN = keyARNExisting
		}
	}

	_, errRotate := client.EnableKeyRotation(ctx, &kms.EnableKeyRotationInput{KeyId: aws.String(keyID)})
	if errRotate != nil && !isAWSErrorCode(errRotate, "UnsupportedOperationException") {
		return "", fmt.Errorf("enable key rotation for kms key %s: %w", keyID, errRotate)
	}

	if keyARN == "" {
		descOut, errDescribe := client.DescribeKey(ctx, &kms.DescribeKeyInput{KeyId: aws.String(keyID)})
		if errDescribe != nil {
			return "", fmt.Errorf("describe kms key %s: %w", keyID, errDescribe)
		}
		if descOut.KeyMetadata == nil {
			return "", fmt.Errorf("describe kms key %s returned empty metadata", keyID)
		}
		keyARN = aws.ToString(descOut.KeyMetadata.Arn)
	}

	if errGrant := ensureKMSGrant(ctx, client, keyID, roleARN, functionName); errGrant != nil {
		return "", errGrant
	}

	return keyARN, nil
}

func ensureKMSGrant(ctx context.Context, client *kms.Client, keyID, roleARN, functionName string) error {
	grantName := functionName + "-lambda-env"
	p := kms.NewListGrantsPaginator(client, &kms.ListGrantsInput{KeyId: aws.String(keyID)})
	for p.HasMorePages() {
		page, errPage := p.NextPage(ctx)
		if errPage != nil {
			return fmt.Errorf("list grants for kms key %s: %w", keyID, errPage)
		}

		for _, g := range page.Grants {
			if aws.ToString(g.GranteePrincipal) == roleARN && aws.ToString(g.Name) == grantName {
				return nil
			}
		}
	}

	_, errGrant := client.CreateGrant(ctx, &kms.CreateGrantInput{
		GranteePrincipal: aws.String(roleARN),
		KeyId:            aws.String(keyID),
		Name:             aws.String(grantName),
		Operations: []kmstypes.GrantOperation{
			kmstypes.GrantOperationDecrypt,
			kmstypes.GrantOperationEncrypt,
			kmstypes.GrantOperationGenerateDataKey,
			kmstypes.GrantOperationGenerateDataKeyWithoutPlaintext,
			kmstypes.GrantOperationDescribeKey,
		},
	})
	if errGrant != nil {
		return fmt.Errorf("create kms grant for role %s on key %s: %w", roleARN, keyID, errGrant)
	}

	return nil
}

func findKMSKeyByAlias(ctx context.Context, client *kms.Client, alias string) (string, string, error) {
	p := kms.NewListAliasesPaginator(client, &kms.ListAliasesInput{})
	for p.HasMorePages() {
		page, errPage := p.NextPage(ctx)
		if errPage != nil {
			return "", "", errPage
		}

		for _, a := range page.Aliases {
			if aws.ToString(a.AliasName) != alias {
				continue
			}

			keyID := aws.ToString(a.TargetKeyId)
			if keyID == "" {
				return "", "", nil
			}

			descOut, errDescribe := client.DescribeKey(ctx, &kms.DescribeKeyInput{KeyId: aws.String(keyID)})
			if errDescribe != nil {
				return "", "", errDescribe
			}
			if descOut.KeyMetadata == nil {
				return keyID, "", nil
			}

			return keyID, aws.ToString(descOut.KeyMetadata.Arn), nil
		}
	}

	return "", "", nil
}

func ensureLambda(ctx context.Context, client *lambdasvc.Client, functionName,
	roleARN string, zipBytes []byte, subnetIDs []string,
	securityGroupID string, functionTimeoutInSeconds,
	memoryInMB int32, architecture, handler, runtime string,
	envVars map[string]string, kmsKeyARN string) error {

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

		log.Printf("get lambda function: name=%s: error: %v", functionName, errGet)

		var notFound *lambdatypes.ResourceNotFoundException
		if errors.As(errGet, &notFound) {

			log.Printf("creating lambda function: name=%s", functionName)

			input := &lambdasvc.CreateFunctionInput{
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
			}

			if kmsKeyARN != "" {
				input.KMSKeyArn = aws.String(kmsKeyARN)
			}

			errCreate := retryOnRoleNotReady(ctx, func() error {
				_, err := client.CreateFunction(ctx, input)
				return err
			})
			if errCreate != nil {
				return fmt.Errorf("create function: %w", errCreate)
			}
			return nil
		}
		return fmt.Errorf("get function: %w", errGet)
	}

	log.Printf("updating lambda function code: name=%s", functionName)

	_, errCode := client.UpdateFunctionCode(ctx, &lambdasvc.UpdateFunctionCodeInput{
		FunctionName: aws.String(functionName),
		ZipFile:      zipBytes,
	})
	if errCode != nil {
		return fmt.Errorf("update function code: %w", errCode)
	}

	// Wait for the code update to complete before updating configuration
	log.Printf("waiting for lambda code update to complete: name=%s", functionName)
	if err := waitForLambdaUpdate(ctx, client, functionName); err != nil {
		return fmt.Errorf("wait for code update: %w", err)
	}

	log.Printf("updating lambda function configuration: name=%s", functionName)

	input := &lambdasvc.UpdateFunctionConfigurationInput{
		FunctionName: aws.String(functionName),
		Description:  aws.String(functionName),
		Handler:      aws.String(handler),
		Role:         aws.String(roleARN),
		Runtime:      lambdatypes.Runtime(runtime),
		VpcConfig:    vpcConfig,
		Timeout:      aws.Int32(functionTimeoutInSeconds),
		MemorySize:   aws.Int32(memoryInMB),
		Environment:  &lambdatypes.Environment{Variables: envVars},
	}

	if kmsKeyARN != "" {
		input.KMSKeyArn = aws.String(kmsKeyARN)
	}

	_, errCfg := client.UpdateFunctionConfiguration(ctx, input)
	if errCfg != nil {
		return fmt.Errorf("update function configuration: %w", errCfg)
	}

	return nil
}

// retryOnRoleNotReady retries the given operation when AWS returns an error indicating the IAM role
// is not yet assumable by Lambda. This is needed because IAM changes are eventually consistent.
func retryOnRoleNotReady(ctx context.Context, op func() error) error {
	const retryInterval = 2 * time.Second
	const maxAttempts = 10
	const roleNotReadyMsg = "The role defined for the function cannot be assumed by Lambda"

	for attempt := range maxAttempts {
		err := op()
		if err == nil {
			return nil
		}
		if !strings.Contains(err.Error(), roleNotReadyMsg) {
			return err
		}
		log.Printf("role not yet ready, retrying (attempt %d/%d)...", attempt+1, maxAttempts)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryInterval):
		}
	}

	return fmt.Errorf("role did not become assumable by Lambda after %d attempts", maxAttempts)
}

// waitForLambdaUpdate polls the Lambda function until it's in Active state with successful update
// This ensures code updates are complete before attempting configuration updates
func waitForLambdaUpdate(ctx context.Context, client *lambdasvc.Client, functionName string) error {
	maxAttempts := 60
	pollInterval := 2 * time.Second

	for attempt := range maxAttempts {
		resp, err := client.GetFunction(ctx, &lambdasvc.GetFunctionInput{
			FunctionName: aws.String(functionName),
		})
		if err != nil {
			return fmt.Errorf("get function: %w", err)
		}

		if resp.Configuration != nil &&
			resp.Configuration.State == lambdatypes.StateActive &&
			resp.Configuration.LastUpdateStatus == lambdatypes.LastUpdateStatusSuccessful {
			log.Printf("lambda function is active with successful update: name=%s", functionName)
			return nil
		}

		log.Printf("lambda function updating (attempt %d/%d): name=%s, state=%v, lastUpdateStatus=%v",
			attempt+1, maxAttempts, functionName, resp.Configuration.State, resp.Configuration.LastUpdateStatus)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	return fmt.Errorf("lambda function did not reach Active state with Successful update within %v seconds", maxAttempts)
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
	kmsClient := kms.NewFromConfig(cfg)
	lambdaClient := lambdasvc.NewFromConfig(cfg)
	logsClient := cloudwatchlogs.NewFromConfig(cfg)

	// delete lambda function

	var errs []error

	if err := deleteLambdaFunction(ctx, lambdaClient, parameters.functionName); err != nil {
		errs = append(errs, err)
	}

	// schedule deletion for kms key protecting lambda environment variables

	if err := deleteKMSKey(ctx, kmsClient, parameters.functionName); err != nil {
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

func deleteKMSKey(ctx context.Context, client *kms.Client, functionName string) error {
	alias := kmsAliasName(functionName)
	log.Printf("deleting kms key: alias=%s", alias)

	keyID, _, errFind := findKMSKeyByAlias(ctx, client, alias)
	if errFind != nil {
		return fmt.Errorf("find kms key by alias %s for deletion: %w", alias, errFind)
	}

	if keyID == "" {
		log.Printf("kms alias/key not found: alias=%s", alias)
		return nil
	}

	_, errAlias := client.DeleteAlias(ctx, &kms.DeleteAliasInput{AliasName: aws.String(alias)})
	if errAlias != nil && !isAWSErrorCode(errAlias, "NotFoundException") {
		return fmt.Errorf("delete kms alias %s: %w", alias, errAlias)
	}

	_, errSchedule := client.ScheduleKeyDeletion(ctx, &kms.ScheduleKeyDeletionInput{
		KeyId:               aws.String(keyID),
		PendingWindowInDays: aws.Int32(7),
	})
	if errSchedule != nil {
		if isAWSErrorCode(errSchedule, "NotFoundException", "KMSInvalidStateException") {
			log.Printf("kms key already missing or pending deletion: key=%s", keyID)
			return nil
		}
		return fmt.Errorf("schedule deletion for kms key %s: %w", keyID, errSchedule)
	}

	log.Printf("scheduled kms key deletion: key=%s pending_days=7", keyID)
	return nil
}

func isAWSErrorCode(err error, codes ...string) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}

	return slices.Contains(codes, apiErr.ErrorCode())
}
