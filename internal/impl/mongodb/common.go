package mongodb

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/writeconcern"

	"github.com/benthosdev/benthos/v4/internal/docs"
	"github.com/benthosdev/benthos/v4/public/bloblang"
	"github.com/benthosdev/benthos/v4/public/service"
)

// JSONMarshalMode represents the way in which BSON should be marshalled to JSON.
type JSONMarshalMode string

const (
	// JSONMarshalModeCanonical Canonical BSON to JSON marshal mode.
	JSONMarshalModeCanonical JSONMarshalMode = "canonical"
	// JSONMarshalModeRelaxed Relaxed BSON to JSON marshal mode.
	JSONMarshalModeRelaxed JSONMarshalMode = "relaxed"
)

//------------------------------------------------------------------------------

const (
	// Common Client Fields
	commonFieldClientURL      = "url"
	commonFieldClientDatabase = "database"
	commonFieldClientUsername = "username"
	commonFieldClientPassword = "password"
)

func clientFields() []*service.ConfigField {
	return []*service.ConfigField{
		service.NewURLField(commonFieldClientURL).
			Description("The URL of the target MongoDB server.").
			Example("mongodb://localhost:27017"),
		service.NewStringField(commonFieldClientDatabase).
			Description("The name of the target MongoDB database."),
		service.NewStringField(commonFieldClientUsername).
			Description("The username to connect to the database.").
			Default(""),
		service.NewStringField(commonFieldClientPassword).
			Description("The password to connect to the database.").
			Default("").
			Secret(),
	}
}

func getClient(parsedConf *service.ParsedConfig) (client *mongo.Client, database *mongo.Database, err error) {
	var url string
	if url, err = parsedConf.FieldString(commonFieldClientURL); err != nil {
		return
	}

	var username, password string
	if username, err = parsedConf.FieldString(commonFieldClientUsername); err != nil {
		return
	}
	if password, err = parsedConf.FieldString(commonFieldClientPassword); err != nil {
		return
	}

	opt := options.Client().
		SetConnectTimeout(10 * time.Second).
		SetSocketTimeout(30 * time.Second).
		SetServerSelectionTimeout(30 * time.Second).
		ApplyURI(url)

	if username != "" && password != "" {
		creds := options.Credential{
			Username: username,
			Password: password,
		}
		opt.SetAuth(creds)
	}

	if client, err = mongo.NewClient(opt); err != nil {
		return
	}

	var databaseStr string
	if databaseStr, err = parsedConf.FieldString(commonFieldClientDatabase); err != nil {
		return
	}

	database = client.Database(databaseStr)
	return
}

//------------------------------------------------------------------------------

// Operation represents the operation that will be performed by MongoDB.
type Operation string

const (
	// OperationInsertOne Insert One operation.
	OperationInsertOne Operation = "insert-one"
	// OperationDeleteOne Delete One operation.
	OperationDeleteOne Operation = "delete-one"
	// OperationDeleteMany Delete many operation.
	OperationDeleteMany Operation = "delete-many"
	// OperationReplaceOne Replace one operation.
	OperationReplaceOne Operation = "replace-one"
	// OperationUpdateOne Update one operation.
	OperationUpdateOne Operation = "update-one"
	// OperationFindOne Find one operation.
	OperationFindOne Operation = "find-one"
	// OperationInvalid Invalid operation.
	OperationInvalid Operation = "invalid"
)

func (op Operation) isDocumentAllowed() bool {
	switch op {
	case OperationInsertOne,
		OperationReplaceOne,
		OperationUpdateOne:
		return true
	default:
		return false
	}
}

func (op Operation) isFilterAllowed() bool {
	switch op {
	case OperationDeleteOne,
		OperationDeleteMany,
		OperationReplaceOne,
		OperationUpdateOne,
		OperationFindOne:
		return true
	default:
		return false
	}
}

func (op Operation) isHintAllowed() bool {
	switch op {
	case OperationDeleteOne,
		OperationDeleteMany,
		OperationReplaceOne,
		OperationUpdateOne,
		OperationFindOne:
		return true
	default:
		return false
	}
}

func (op Operation) isUpsertAllowed() bool {
	switch op {
	case OperationReplaceOne,
		OperationUpdateOne:
		return true
	default:
		return false
	}
}

// NewOperation converts a string operation to a strongly-typed Operation.
func NewOperation(op string) Operation {
	switch op {
	case "insert-one":
		return OperationInsertOne
	case "delete-one":
		return OperationDeleteOne
	case "delete-many":
		return OperationDeleteMany
	case "replace-one":
		return OperationReplaceOne
	case "update-one":
		return OperationUpdateOne
	case "find-one":
		return OperationFindOne
	default:
		return OperationInvalid
	}
}

const (
	// Common Operation Fields
	commonFieldOperation = "operation"
)

func processorOperationDocs(defaultOperation Operation) docs.FieldSpec {
	fs := outputOperationDocs(defaultOperation)
	return fs.HasOptions(append(fs.Options, string(OperationFindOne))...)
}

func outputOperationDocs(defaultOperation Operation) docs.FieldSpec {
	return docs.FieldString(
		"operation",
		"The mongodb operation to perform.",
	).HasOptions(
		string(OperationInsertOne),
		string(OperationDeleteOne),
		string(OperationDeleteMany),
		string(OperationReplaceOne),
		string(OperationUpdateOne),
	).HasDefault(string(defaultOperation))
}

func operationFromParsed(pConf *service.ParsedConfig) (operation Operation, err error) {
	var operationStr string
	if operationStr, err = pConf.FieldString(commonFieldOperation); err != nil {
		return
	}

	if operation = NewOperation(operationStr); operation == OperationInvalid {
		err = fmt.Errorf("mongodb operation '%s' unknown: must be insert-one, delete-one, delete-many, replace-one or update-one", operationStr)
	}
	return
}

//------------------------------------------------------------------------------

const (
	// Common Write Concern Fields
	commonFieldWriteConcern         = "write_concern"
	commonFieldWriteConcernW        = "w"
	commonFieldWriteConcernJ        = "j"
	commonFieldWriteConcernWTimeout = "w_timeout"
)

func writeConcernDocs() docs.FieldSpec {
	return docs.FieldObject(commonFieldWriteConcern, "The write concern settings for the mongo connection.").
		WithChildren(
			docs.FieldString(commonFieldWriteConcernW, "W requests acknowledgement that write operations propagate to the specified number of mongodb instances.").HasDefault(""),
			docs.FieldBool(commonFieldWriteConcernJ, "J requests acknowledgement from MongoDB that write operations are written to the journal.").HasDefault(false),
			docs.FieldString(commonFieldWriteConcernWTimeout, "The write concern timeout.").HasDefault(""),
		)
}

func writeConcernCollectionOptionFromParsed(pConf *service.ParsedConfig) (opt *options.CollectionOptions, err error) {
	pConf = pConf.Namespace(commonFieldWriteConcern)

	var w string
	if w, err = pConf.FieldString(commonFieldWriteConcernW); err != nil {
		return
	}

	var j bool
	if j, err = pConf.FieldBool(commonFieldWriteConcernJ); err != nil {
		return
	}

	var wTimeout time.Duration
	if dStr, _ := pConf.FieldString(commonFieldWriteConcernWTimeout); dStr != "" {
		if wTimeout, err = pConf.FieldDuration(commonFieldWriteConcernWTimeout); err != nil {
			return
		}
	}

	writeConcern := writeconcern.New(
		writeconcern.J(j),
		writeconcern.WTimeout(wTimeout),
	)

	if wInt, err := strconv.Atoi(w); err != nil {
		writeconcern.WTagSet(w)
	} else {
		writeconcern.W(wInt)(writeConcern)
	}

	// This does some validation so we don't have to
	if _, _, err = writeConcern.MarshalBSONValue(); err != nil {
		return nil, fmt.Errorf("write concern validation error: %w", err)
	}

	return options.Collection().SetWriteConcern(writeConcern), nil
}

//------------------------------------------------------------------------------

const (
	// Common Write Map Fields
	commonFieldDocumentMap = "document_map"
	commonFieldFilterMap   = "filter_map"
	commonFieldHintMap     = "hint_map"
	commonFieldUpsert      = "upsert"
)

func writeMapsFields() []*service.ConfigField {
	return []*service.ConfigField{
		service.NewBloblangField(commonFieldDocumentMap).
			Description("A bloblang map representing the records in the mongo db. Used to generate the document for mongodb by " +
				"mapping the fields in the message to the mongodb fields. The document map is required for the operations " +
				"insert-one, replace-one and update-one.").
			Examples(mapExamples()...).
			Default(""),
		service.NewBloblangField(commonFieldFilterMap).
			Description("A bloblang map representing the filter for the mongo db command. The filter map is required for all operations except " +
				"insert-one. It is used to find the document(s) for the operation. For example in a delete-one case, the filter map should " +
				"have the fields required to locate the document to delete.").
			Examples(mapExamples()...).
			Default(""),
		service.NewBloblangField(commonFieldHintMap).
			Description("A bloblang map representing the hint for the mongo db command. This map is optional and is used with all operations " +
				"except insert-one. It is used to improve performance of finding the documents in the mongodb.").
			Examples(mapExamples()...).
			Default(""),
		service.NewBoolField(commonFieldUpsert).
			Description("The upsert setting is optional and only applies for update-one and replace-one operations. If the filter specified in filter_map matches, the document is updated or replaced accordingly, otherwise it is created.").
			Version("3.60.0").
			Default(false),
	}
}

type writeMaps struct {
	filterMap   *bloblang.Executor
	documentMap *bloblang.Executor
	hintMap     *bloblang.Executor
	upsert      bool
}

func writeMapsFromParsed(conf *service.ParsedConfig, operation Operation) (maps writeMaps, err error) {
	if probeStr, _ := conf.FieldString(commonFieldFilterMap); probeStr != "" {
		if maps.filterMap, err = conf.FieldBloblang(commonFieldFilterMap); err != nil {
			return
		}
	}
	if probeStr, _ := conf.FieldString(commonFieldDocumentMap); probeStr != "" {
		if maps.documentMap, err = conf.FieldBloblang(commonFieldDocumentMap); err != nil {
			return
		}
	}
	if probeStr, _ := conf.FieldString(commonFieldHintMap); probeStr != "" {
		if maps.hintMap, err = conf.FieldBloblang(commonFieldHintMap); err != nil {
			return
		}
	}
	if maps.upsert, err = conf.FieldBool(commonFieldUpsert); err != nil {
		return
	}

	if operation.isFilterAllowed() {
		if maps.filterMap == nil {
			err = errors.New("mongodb filter_map must be specified")
			return
		}
	} else if maps.filterMap != nil {
		err = fmt.Errorf("mongodb filter_map not allowed for '%s' operation", operation)
		return
	}

	if operation.isDocumentAllowed() {
		if maps.documentMap == nil {
			err = errors.New("mongodb document_map must be specified")
			return
		}
	} else if maps.documentMap != nil {
		err = fmt.Errorf("mongodb document_map not allowed for '%s' operation", operation)
		return
	}

	if !operation.isHintAllowed() && maps.hintMap != nil {
		err = fmt.Errorf("mongodb hint_map not allowed for '%s' operation", operation)
		return
	}

	if !operation.isUpsertAllowed() && maps.upsert {
		err = fmt.Errorf("mongodb upsert not allowed for '%s' operation", operation)
		return
	}

	return
}

func (w writeMaps) extractFromMessage(operation Operation, i int, batch service.MessageBatch) (
	docJSON, filterJSON, hintJSON any, err error,
) {
	var hintVal, filterVal, documentVal *service.Message
	var filterValWanted, documentValWanted bool

	filterValWanted = operation.isFilterAllowed()
	documentValWanted = operation.isDocumentAllowed()

	if filterValWanted {
		if filterVal, err = batch.BloblangQuery(i, w.filterMap); err != nil {
			err = fmt.Errorf("failed to execute filter_map: %v", err)
			return
		}
	}

	if (filterVal != nil || !filterValWanted) && documentValWanted {
		if documentVal, err = batch.BloblangQuery(i, w.documentMap); err != nil {
			err = fmt.Errorf("failed to execute document_map: %v", err)
			return
		}
	}

	if filterVal == nil && filterValWanted {
		err = fmt.Errorf("failed to generate filterVal")
		return
	}

	if documentVal == nil && documentValWanted {
		err = fmt.Errorf("failed to generate documentVal")
		return
	}

	if filterValWanted {
		if filterJSON, err = filterVal.AsStructured(); err != nil {
			return
		}
	}

	if documentValWanted {
		if docJSON, err = documentVal.AsStructured(); err != nil {
			return
		}
	}

	if w.hintMap != nil {
		hintVal, err = batch.BloblangQuery(i, w.hintMap)
		if err != nil {
			err = fmt.Errorf("failed to execute hint_map: %v", err)
			return
		}
		if hintJSON, err = hintVal.AsStructured(); err != nil {
			return
		}
	}
	return
}

func mapExamples() []any {
	examples := []any{"root.a = this.foo\nroot.b = this.bar"}
	return examples
}
