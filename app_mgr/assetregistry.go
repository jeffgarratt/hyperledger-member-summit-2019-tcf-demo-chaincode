/*
Copyright IBM Corp. All Rights Reserved.

SPDX-License-Identifier: Apache-2.0
*/

package main

import (
	"fmt"

	"github.com/hyperledger/fabric/core/chaincode/shim"
	sc "github.com/hyperledger/fabric/protos/peer"
	pb "github.com/hyperledger/fabric/protos/peer"
	"github.com/golang/protobuf/proto"
)

var COMPOSITE_KEY_APP_BUNDLE_OBJECTTYPE string = Query_APP_BUNDLE.String()
var COMPOSITE_KEY_APP_DESCRIPTOR_OBJECTTYPE = Query_APP_DESCRIPTOR.String()

// AssetRegistry defines the smart contract structure.
type AssetRegistry struct{}

// Init is called when the chaincode is instantiatied, for now, a no-op
func (s *AssetRegistry) Init(stub shim.ChaincodeStubInterface) sc.Response {
	_ = &pb.SignedChaincodeDeploymentSpec{}
	return shim.Success(nil)
}

// Invoke allows for the manipulation of assets.
// Possible arguments are:
//   ["createAppDescriptor",   <app_key>, <app_descriptor>]                 // Creates a new asset
//   ["createAppBundle",   <app_bundle_key>,  <app_bundle>]                 // Creates a new asset
//   ["associateDescriptorWithBundle", <app_key>, <app_bundle_key>]                 // Associates an AppBundle with an AppDescriptor
//   ["getAppDescriptors", <query>]  // Queries the AppDescriptors
//   ["getAppBundleKeySetForDescriptor", <app_descriptor_key>]
//   ["getAppBundleForDescriptor",<app_descriptor_key>, <app_bundle_key>]
func (s *AssetRegistry) Invoke(stub shim.ChaincodeStubInterface) sc.Response {
	fmt.Printf("Constructing assetContext...\n")
	ac, err := newAssetContext(stub)
	fmt.Printf("Constructed assetContext, assetContext = %v, err = %s\n", ac, err)

	if err != nil {
		return shim.Error(err.Error())
	}
	return ac.execute()
}

// parseArgs returns the function name, the key of the asset to operate on, an optional
// additional arg for the function, or an error if there are too few, or too many args.
func parseArgs(args [][]byte) (function string, key string, arg []byte, err error) {
	switch len(args) {
	case 3:
		arg = args[2]
		fallthrough
	case 2:
		key = string(args[1])
		function = string(args[0])
	case 1:
		err = fmt.Errorf("Invoke called with only one argument")
	case 0:
		err = fmt.Errorf("Invoke called with no arguments")
	default:
		err = fmt.Errorf("Invoke called with too many arguments")
	}
	return
}

type assetContext struct {
	stub        shim.ChaincodeStubInterface
	creator     []byte // Guaranteed to be set
	function    string // The name of the operation being invoked
}

func newAssetContext(stub shim.ChaincodeStubInterface) (*assetContext, error) {
	var args = stub.GetArgs()
	var err error = nil
	var function = ""
	switch len(args) {
	case 3:
		fallthrough
	case 2:
		fallthrough
	case 1:
		function = string(args[0])
	case 0:
		err = fmt.Errorf("Invoke called with no arguments")
	default:
		err = fmt.Errorf("Invoke called with too many arguments")
	}
	if err != nil {
		return nil, err
	}

	creator, err := stub.GetCreator()
	if err != nil {
		return nil, fmt.Errorf("Could not get creator: %s", err)
	}

	return &assetContext{
		stub:        stub,
		creator:     creator,
		function:    function,
	}, nil
}

func (ac *assetContext) execute() sc.Response {
	// Route to the appropriate handler function to interact with the ledger appropriately
	var err error
	var result []byte

	fmt.Printf("inside execute... function = %s\n", ac.function)
	switch ac.function {
	case "createAppDescriptor":
		result, err = ac.createAppDescriptor()
	case "createAppBundle":
		result, err = ac.createAppBundle()
	case "associateDescriptorWithBundle":
		result, err = ac.associateDescriptorWithBundle()
	case "getAppDescriptors":
		result, err = ac.getAppDescriptors()
	case "getAppBundleKeySetForDescriptor":
		result, err = ac.getAppBundleKeySetForDescriptor()
	case "getAppBundleForDescriptor":
		result, err = ac.getAppBundleForDescriptor()
	default:
		return shim.Error("Invalid invocation function")
	}

	if err != nil {
		return shim.Error(err.Error())
	}

	return shim.Success(result)
}

func (ac *assetContext) getDescriptor(key_part string) (*AppDescriptor, error){
	compositeKey, err := ac.stub.CreateCompositeKey(COMPOSITE_KEY_APP_DESCRIPTOR_OBJECTTYPE, []string{key_part})
	if err != nil {
		return nil, fmt.Errorf("Error creating composite key_part for %s using base component (%s):  %s", COMPOSITE_KEY_APP_BUNDLE_OBJECTTYPE, key_part, err)
	}

	appDescriptorBytesFromStore, err := ac.stub.GetState(compositeKey)
	if appDescriptorBytesFromStore == nil {
		return nil, fmt.Errorf("AppDescriptor not found for key_part %s", key_part)
	}

	appDescriptor := &AppDescriptor{}
	if err := proto.Unmarshal(appDescriptorBytesFromStore, appDescriptor); err != nil {
		return nil, fmt.Errorf("Cannot unmarshal AppDescriptor, err = %s", err.Error())
	}
	return appDescriptor, nil
}

func (ac *assetContext) createAppDescriptor() ([]byte, error) {
	var args = ac.stub.GetArgs()
	key_part := ""

	var appDescriptorBytesFromArgs = []byte{}
	switch len(args) {
	case 3:
		key_part = string(args[1])
		appDescriptorBytesFromArgs = args[2]
	default:
		return nil, fmt.Errorf("Wrong number of arguments to createAppDescriptor")
	}

	compositeKey, err := ac.stub.CreateCompositeKey(COMPOSITE_KEY_APP_DESCRIPTOR_OBJECTTYPE, []string{key_part})
	if err != nil {
		return nil, fmt.Errorf("Error creating composite key_part for %s using base component (%s):  %s", COMPOSITE_KEY_APP_BUNDLE_OBJECTTYPE, key_part, err)
	}


	appDescriptorBytesFromStore, err := ac.stub.GetState(compositeKey)
	if appDescriptorBytesFromStore != nil {
		return nil, fmt.Errorf("Cannot create an AppDescriptor whose key_part already exists")
	}

	appDescriptor := &AppDescriptor{}
	if err := proto.Unmarshal(appDescriptorBytesFromArgs, appDescriptor); err != nil {
		return nil, fmt.Errorf("Cannot unmarshal AppDescriptor, err = %s", err.Error())
	}
	// Make sure bundle_id is NOT set
	if len(appDescriptor.BundleId) != 0 {
		return nil, fmt.Errorf("AppDscriptor's bundle_id field must be empty during creation")
	}

	// Set the owner if not set
	if len(appDescriptor.Owner) == 0 {
		appDescriptor.Owner = ac.creator
	}

	appDescriptorBytesToStore, err := proto.Marshal(appDescriptor)
	if err != nil {
		return nil, fmt.Errorf("Error marshaling proto: %s", err)
	}

	err = ac.stub.PutState(compositeKey, appDescriptorBytesToStore)
	if err != nil {
		return nil, fmt.Errorf("Could not put state for key %s: %s", compositeKey, err)
	}

	return appDescriptorBytesToStore, nil
}


func (ac *assetContext) createAppBundle() ([]byte, error) {
	var args = ac.stub.GetArgs()
	key_part := ""

	var appBundleBytesFromArgs = []byte{}
	switch len(args) {
	case 3:
		key_part = string(args[1])
		appBundleBytesFromArgs = args[2]
	default:
		return nil, fmt.Errorf("Wrong number of arguments to createAppBundle")
	}

	// First get the AppBundle from the args
	appBundle := &AppBundle{}
	if err := proto.Unmarshal(appBundleBytesFromArgs, appBundle); err != nil {
		return nil, fmt.Errorf("Cannot unmarshal AppBundle, err = %s", err.Error())
	}

	if len(appBundle.Artifacts) == 0 && len(appBundle.ChaincodeDeploymentSpecs) == 0 {
		return nil, fmt.Errorf("Must specify at least 1 artifact or chaincode deployment spec in an AppBundle")
	}

	// Set the owner if not set
	if len(appBundle.Owner) == 0 {
		appBundle.Owner = ac.creator
	}

	// Make sure the descriptor exists
	_, err := ac.getDescriptor(appBundle.DescriptorId)
	if err != nil {
		return nil, fmt.Errorf("Could not get descriptor for AppBundle with descriptor_id = %s:  %s", appBundle.DescriptorId, err.Error())
	}

	// Get the composite key_part
	compositeKey, err := ac.stub.CreateCompositeKey(COMPOSITE_KEY_APP_BUNDLE_OBJECTTYPE, []string{appBundle.DescriptorId, key_part})
	if err != nil {
		return nil, fmt.Errorf("Error creating composite key_part for %s using base component (%s):  %s", COMPOSITE_KEY_APP_BUNDLE_OBJECTTYPE, key_part, err)
	}

	appBundleBytesFromStore, err := ac.stub.GetState(compositeKey)
	if appBundleBytesFromStore != nil {
		return nil, fmt.Errorf("Cannot create an AppBundle whose key_part already exists: %s", compositeKey)
	}

	appBundleBytes, err := proto.Marshal(appBundle)
	if err != nil {
		return nil, fmt.Errorf("Error marshaling proto: %s", err)
	}

	err = ac.stub.PutState(compositeKey, appBundleBytes)
	if err != nil {
		return nil, fmt.Errorf("Could not put state for key_part %s: %s", compositeKey, err)
	}

	return appBundleBytes, nil
}


func (ac *assetContext) associateDescriptorWithBundle() ([]byte, error) {
	var args = ac.stub.GetArgs()
	app_descriptor_key_part := ""
	app_bundle_key_part := ""

	switch len(args) {
	case 3:
		app_descriptor_key_part = string(args[1])
		app_bundle_key_part = string(args[2])
	default:
		return nil, fmt.Errorf("Wrong number of arguments to associateDescriptorWithBundle")
	}


	// Verify AppDescriptor exists
	appDescriptor, err := ac.getDescriptor(app_descriptor_key_part)
	if err != nil {
		return nil, fmt.Errorf("Error in associateDescriptorWithBundle: %s", err.Error())
	}

	// Verify AppBundle exists
	_, err = ac.getAppBundleForDescriptorByKey(app_descriptor_key_part, app_bundle_key_part)
	if err != nil {
		return nil, fmt.Errorf("Error in associateDescriptorWithBundle: %s", err.Error())
	}

	// Now set the bundle_id field on
	appDescriptor.BundleId = app_bundle_key_part
	appDescriptorBytesToStore, err := proto.Marshal(appDescriptor)
	if err != nil {
		return nil, fmt.Errorf("Error in associateDescriptorWithBundle, error marshaling proto: %s", err)
	}

	app_descriptor_composite_key, err := ac.stub.CreateCompositeKey(COMPOSITE_KEY_APP_DESCRIPTOR_OBJECTTYPE, []string{app_descriptor_key_part})
	if err != nil {
		return nil, fmt.Errorf("Error in associateDescriptorWithBundle, could not create app_descriptor composite key: %s", err.Error())
	}
	err = ac.stub.PutState(app_descriptor_composite_key, appDescriptorBytesToStore)
	if err != nil {
		return nil, fmt.Errorf("Error in associateDescriptorWithBundle, could not put state for AppDescriptor key %s: %s", app_descriptor_key_part, err)
	}

	return appDescriptorBytesToStore, nil
}


func (ac *assetContext) getAppBundleForDescriptorByKey(app_descriptor_key string, app_bundle_key string) ([]byte, error){
	var key_parts = []string{app_descriptor_key, app_bundle_key}
	compositeKey, err := ac.stub.CreateCompositeKey(COMPOSITE_KEY_APP_BUNDLE_OBJECTTYPE, key_parts)
	if err != nil {
		return nil, fmt.Errorf("Error creating composite app_bundle_key for object_type (%s) and key_parts (%v):  %s", COMPOSITE_KEY_APP_BUNDLE_OBJECTTYPE, key_parts, err)
	}

	appBundleBytesFromStore, err := ac.stub.GetState(compositeKey)
	if err != nil {
		return nil, fmt.Errorf("Error in GetState using composite key (%v) for getAppBundleForDescriptorByKey: %s", compositeKey, err.Error())
	}
	if appBundleBytesFromStore == nil {
		return nil, fmt.Errorf("Error in getAppBundleForDescriptorByKey for composite key (%v), AppBundle not found.", compositeKey)
	}

	return appBundleBytesFromStore, nil
}


func (ac *assetContext) query(query *Query) (*QueryResult, error) {
	fmt.Printf("Entering query function\n")
	stateQueryIterator, err := ac.stub.GetStateByPartialCompositeKey(query.ObjectType.String(), query.KeyParts)
	if err != nil {
		return nil, fmt.Errorf("Error in query using object_type = %s and query %v: %s", query.ObjectType.String(), query, err)
	}
	defer stateQueryIterator.Close()

	var queryResult = &QueryResult{Query: query, Results: make(map[string][]byte)}
	for stateQueryIterator.HasNext() {
		queryResultFromIterator, err := stateQueryIterator.Next()
		if (err != nil) {
			return nil, fmt.Errorf("Error in query using Query = (%v): %s", query, err)
		}
		_, key_parts, err := ac.stub.SplitCompositeKey(queryResultFromIterator.Key)
		last_key_part := key_parts[len(key_parts)-1]
		if err != nil {
			return nil, fmt.Errorf("Error in query, could not split returned composite key using Query = (%v): %s", query, err)
		}
		queryResult.Results[last_key_part] = queryResultFromIterator.Value
	}
	return queryResult, nil
}

func (ac *assetContext) getAppDescriptors() ([]byte, error) {
	var query *Query = &Query{ObjectType:Query_APP_DESCRIPTOR}
	var query_results, err = ac.query(query)
	if err != nil {
		return nil, fmt.Errorf("Error in getAppDescriptors: %s", err)
	}
	var appDescriptors = &AppDescriptors{Descriptors:make(map[string]*AppDescriptor)}
	for k, v := range query_results.Results {
		var appDescriptor = &AppDescriptor{}
		if err := proto.Unmarshal(v, appDescriptor); err != nil {
			return nil, fmt.Errorf("Error unmarshalling AppDescriptor in getAppDescriptors for key '%s': %s", k, err)
		}
		appDescriptors.Descriptors[k] = appDescriptor
	}
	var appDescriptorsBytes, err_marshalling = proto.Marshal(appDescriptors)
	if err_marshalling != nil {
		return nil, fmt.Errorf("Error marshalling AppDescriptors in getAppDescriptors: %s", err_marshalling.Error())
	}
	return appDescriptorsBytes, nil
}


func (ac *assetContext) getAppBundleKeySetForDescriptor() ([]byte, error) {
	var args = ac.stub.GetArgs()
	app_descriptor_key_part := ""

	switch len(args) {
	case 2:
		app_descriptor_key_part = string(args[1])
	default:
		return nil, fmt.Errorf("Wrong number of arguments to getAppBundleKeySetForDescriptor")
	}

	// First make sure descriptor exists
	_, err_get_descriptor := ac.getDescriptor(app_descriptor_key_part)
	if err_get_descriptor != nil {
		return nil, fmt.Errorf("Error trying to get app_descriptor (%s) inside getAppBundleKeySetForDescriptor: %s", app_descriptor_key_part, err_get_descriptor.Error())
	}

	var query *Query = &Query{ObjectType:Query_APP_BUNDLE, KeyParts: []string{app_descriptor_key_part}}
	var query_results, err = ac.query(query)
	if err != nil {
		return nil, fmt.Errorf("Error in getAppBundleKeySetForDescriptor: %s", err.Error())
	}
	var appBundleKeySet = &AppBundleKeySet{DescriptorId: app_descriptor_key_part}
	for k, _ := range query_results.Results {
		appBundleKeySet.BundleKeys = append(appBundleKeySet.BundleKeys, k)
	}
	var appBundleKeySetBytes, err_marshalling = proto.Marshal(appBundleKeySet)
	if err_marshalling != nil {
		return nil, fmt.Errorf("Error marshalling AppBundleKeySet in getAppBundleKeySetForDescriptor: %s", err_marshalling.Error())
	}
	return appBundleKeySetBytes, nil
}




func (ac *assetContext) getAppBundleForDescriptor() ([]byte, error) {
	var args = ac.stub.GetArgs()
	app_descriptor_key_part := ""
	app_bundle_key_part := ""

	switch len(args) {
	case 3:
		app_descriptor_key_part = string(args[1])
		app_bundle_key_part = string(args[2])
	default:
		return nil, fmt.Errorf("Wrong number of arguments to getAppBundleForDescriptor")
	}

	// Verify AppDescriptor exists
	_, err := ac.getDescriptor(app_descriptor_key_part)
	if err != nil {
		return nil, fmt.Errorf("Error in getAppBundleForDescriptor: %s", err.Error())
	}

	// Verify AppBundle exists
	appBundleBytesFromStore, err := ac.getAppBundleForDescriptorByKey(app_descriptor_key_part, app_bundle_key_part)
	if err != nil {
		return nil, fmt.Errorf("Error in getAppBundleForDescriptor: %s", err.Error())
	}
	return appBundleBytesFromStore, nil
}

// main function starts up the chaincode in the container during instantiate
func main() {
	if err := shim.Start(new(AssetRegistry)); err != nil {
		fmt.Printf("Error starting AssetRegistry chaincode: %s\n", err)
	}
}


