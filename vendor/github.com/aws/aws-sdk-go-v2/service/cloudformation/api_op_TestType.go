// Code generated by smithy-go-codegen DO NOT EDIT.

package cloudformation

import (
	"context"
	"fmt"
	awsmiddleware "github.com/aws/aws-sdk-go-v2/aws/middleware"
	"github.com/aws/aws-sdk-go-v2/service/cloudformation/types"
	"github.com/aws/smithy-go/middleware"
	smithyhttp "github.com/aws/smithy-go/transport/http"
)

// Tests a registered extension to make sure it meets all necessary requirements
// for being published in the CloudFormation registry.
//   - For resource types, this includes passing all contracts tests defined for
//     the type.
//   - For modules, this includes determining if the module's model meets all
//     necessary requirements.
//
// For more information, see Testing your public extension prior to publishing (https://docs.aws.amazon.com/cloudformation-cli/latest/userguide/publish-extension.html#publish-extension-testing)
// in the CloudFormation CLI User Guide. If you don't specify a version,
// CloudFormation uses the default version of the extension in your account and
// Region for testing. To perform testing, CloudFormation assumes the execution
// role specified when the type was registered. For more information, see
// RegisterType (https://docs.aws.amazon.com/AWSCloudFormation/latest/APIReference/API_RegisterType.html)
// . Once you've initiated testing on an extension using TestType , you can pass
// the returned TypeVersionArn into DescribeType (https://docs.aws.amazon.com/AWSCloudFormation/latest/APIReference/API_DescribeType.html)
// to monitor the current test status and test status description for the
// extension. An extension must have a test status of PASSED before it can be
// published. For more information, see Publishing extensions to make them
// available for public use (https://docs.aws.amazon.com/cloudformation-cli/latest/userguide/resource-type-publish.html)
// in the CloudFormation CLI User Guide.
func (c *Client) TestType(ctx context.Context, params *TestTypeInput, optFns ...func(*Options)) (*TestTypeOutput, error) {
	if params == nil {
		params = &TestTypeInput{}
	}

	result, metadata, err := c.invokeOperation(ctx, "TestType", params, optFns, c.addOperationTestTypeMiddlewares)
	if err != nil {
		return nil, err
	}

	out := result.(*TestTypeOutput)
	out.ResultMetadata = metadata
	return out, nil
}

type TestTypeInput struct {

	// The Amazon Resource Name (ARN) of the extension. Conditional: You must specify
	// Arn , or TypeName and Type .
	Arn *string

	// The S3 bucket to which CloudFormation delivers the contract test execution
	// logs. CloudFormation delivers the logs by the time contract testing has
	// completed and the extension has been assigned a test type status of PASSED or
	// FAILED . The user calling TestType must be able to access items in the
	// specified S3 bucket. Specifically, the user needs the following permissions:
	//   - GetObject
	//   - PutObject
	// For more information, see Actions, Resources, and Condition Keys for Amazon S3 (https://docs.aws.amazon.com/service-authorization/latest/reference/list_amazons3.html)
	// in the Amazon Web Services Identity and Access Management User Guide.
	LogDeliveryBucket *string

	// The type of the extension to test. Conditional: You must specify Arn , or
	// TypeName and Type .
	Type types.ThirdPartyType

	// The name of the extension to test. Conditional: You must specify Arn , or
	// TypeName and Type .
	TypeName *string

	// The version of the extension to test. You can specify the version id with
	// either Arn , or with TypeName and Type . If you don't specify a version,
	// CloudFormation uses the default version of the extension in this account and
	// Region for testing.
	VersionId *string

	noSmithyDocumentSerde
}

type TestTypeOutput struct {

	// The Amazon Resource Name (ARN) of the extension.
	TypeVersionArn *string

	// Metadata pertaining to the operation's result.
	ResultMetadata middleware.Metadata

	noSmithyDocumentSerde
}

func (c *Client) addOperationTestTypeMiddlewares(stack *middleware.Stack, options Options) (err error) {
	if err := stack.Serialize.Add(&setOperationInputMiddleware{}, middleware.After); err != nil {
		return err
	}
	err = stack.Serialize.Add(&awsAwsquery_serializeOpTestType{}, middleware.After)
	if err != nil {
		return err
	}
	err = stack.Deserialize.Add(&awsAwsquery_deserializeOpTestType{}, middleware.After)
	if err != nil {
		return err
	}
	if err := addProtocolFinalizerMiddlewares(stack, options, "TestType"); err != nil {
		return fmt.Errorf("add protocol finalizers: %v", err)
	}

	if err = addlegacyEndpointContextSetter(stack, options); err != nil {
		return err
	}
	if err = addSetLoggerMiddleware(stack, options); err != nil {
		return err
	}
	if err = addClientRequestID(stack); err != nil {
		return err
	}
	if err = addComputeContentLength(stack); err != nil {
		return err
	}
	if err = addResolveEndpointMiddleware(stack, options); err != nil {
		return err
	}
	if err = addComputePayloadSHA256(stack); err != nil {
		return err
	}
	if err = addRetry(stack, options); err != nil {
		return err
	}
	if err = addRawResponseToMetadata(stack); err != nil {
		return err
	}
	if err = addRecordResponseTiming(stack); err != nil {
		return err
	}
	if err = addClientUserAgent(stack, options); err != nil {
		return err
	}
	if err = smithyhttp.AddErrorCloseResponseBodyMiddleware(stack); err != nil {
		return err
	}
	if err = smithyhttp.AddCloseResponseBodyMiddleware(stack); err != nil {
		return err
	}
	if err = addSetLegacyContextSigningOptionsMiddleware(stack); err != nil {
		return err
	}
	if err = stack.Initialize.Add(newServiceMetadataMiddleware_opTestType(options.Region), middleware.Before); err != nil {
		return err
	}
	if err = addRecursionDetection(stack); err != nil {
		return err
	}
	if err = addRequestIDRetrieverMiddleware(stack); err != nil {
		return err
	}
	if err = addResponseErrorMiddleware(stack); err != nil {
		return err
	}
	if err = addRequestResponseLogging(stack, options); err != nil {
		return err
	}
	if err = addDisableHTTPSMiddleware(stack, options); err != nil {
		return err
	}
	return nil
}

func newServiceMetadataMiddleware_opTestType(region string) *awsmiddleware.RegisterServiceMetadata {
	return &awsmiddleware.RegisterServiceMetadata{
		Region:        region,
		ServiceID:     ServiceID,
		OperationName: "TestType",
	}
}