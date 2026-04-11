// Package main is the entry point for the aws-lambda-httpclient-deploy command-line tool.
package main

import "flag"

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

	flag.StringVar(&parameters.functionName, "function-name", "", "The name of the AWS Lambda function")
	flag.StringVar(&parameters.vpcID, "vpc-id", "", "The ID of the VPC to deploy the Lambda function in")
	flag.StringVar(&parameters.subnetIDs, "subnet-ids", "", "Space-separated IDs of the subnets to deploy the Lambda function in")
	flag.StringVar(&parameters.roleInlinePolicyFile, "role-inline-policy-file", "samples/lambda-role-policy.json", "Path to a JSON file containing the inline policy for the Lambda execution role")
	flag.StringVar(&parameters.zipFile, "zip-file", "lambda.zip", "Path to the ZIP file containing the Lambda function code")
	flag.IntVar(&parameters.logRetentionDays, "log-retention-days", 7, "Number of days to retain logs in CloudWatch")
	flag.BoolVar(&parameters.destroy, "destroy", false, "Whether to destroy the deployed Lambda function")

	flag.Parse()

	if parameters.functionName == "" {
		flag.Usage()
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
	// VPC and subnets we do not create, they are input parameters

	// security group: create or update.
	// the security group will have a single outbout rule allowing both 80 and 443 to anywhere, and no inbound rules.

	// role: create or update.
	// the role will have a single inline policy, which is read from the file specified by the role-inline-policy-file parameter.
	// the role must have permissions needed to log into cloudwatch and to access the VPC and subnets.

	// lambda: create or update.
	// the lambda will take the appropriate specification for Go:
	// - runtime: AL2023
	// - handler: bootstrap
	// - architecture: arm64
	// - code from file lambda.zip

	// create or update the cloudwatch log group for the lambda function.
	// the log group will be named /aws/lambda/<function-name>.
	// the main point is to set the retention policy to 7 days.
}

// destroyLambda deletes the AWS Lambda function and its associated resources based on the provided parameters.
func destroyLambda(parameters lambda) {
	// destroy the lambda function.
	// destroy the role.
	// destroy the security group.
	// destroy the cloudwatch log group for the lambda function.
}
