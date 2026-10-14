package checkpoint

import (
	"encoding/json"
	"fmt"

	"github.com/TimLai666/coimnet/learning"
)

// bundleDocumentPresenceApplies keeps the low-level bundle reader's generic
// array-container contract intact while enforcing presence on the four public
// bundle document types. The public loaders always pass these exact pointers;
// internal callers may use readBundle for format-only round-trip tests.
func bundleDocumentPresenceApplies(kind, schema string, value any) bool {
	switch kind {
	case BundleIndividual:
		_, ok := value.(*learning.IndividualSnapshot)
		return ok && schema == IndividualSchemaVersion
	case BundleTraining:
		_, ok := value.(*learning.TrainingSnapshot)
		return ok && schema == TrainingSchemaVersion
	case BundleModelPackage:
		_, ok := value.(*ModelPackage)
		return ok && schema == ModelPackageSchemaVersion
	case BundleTaskTraining:
		_, ok := value.(*TaskTrainingSnapshot)
		return ok
	default:
		return false
	}
}

// checkBundleDocumentPresence applies the payload-specific raw presence rules
// before a bundle document reaches encoding/json or array restoration. Bundle
// documents contain the payload directly, while the existing single-file
// walkers also require their versioned envelope, so the two envelope-based
// walkers receive a validation-only wrapper below.
func checkBundleDocumentPresence(kind string, data []byte) error {
	switch kind {
	case BundleIndividual:
		if err := checkUniqueJSONRejectNull(data); err != nil {
			return err
		}
		return requireBundleIndividualFields(data)
	case BundleTraining:
		return checkTrainingSnapshotRequiredFields(data)
	case BundleModelPackage:
		if err := checkUniqueJSONRejectNull(data); err != nil {
			return err
		}
		return requireBundleModelPackageFields(data)
	case BundleTaskTraining:
		return checkTaskTrainingDocumentFields(data)
	default:
		return fmt.Errorf("unsupported bundle kind %q", kind)
	}
}

func requireBundleIndividualFields(data []byte) error {
	return requireBundlePayloadFields(IndividualSchemaVersion, data, requireIndividualFields)
}

func requireBundleModelPackageFields(data []byte) error {
	return requireBundlePayloadFields(ModelPackageSchemaVersion, data, requireModelPackageFields)
}

func requireBundlePayloadFields(schema string, payload []byte, validate func([]byte) error) error {
	envelope, err := json.Marshal(struct {
		SchemaVersion string          `json:"schema_version"`
		Payload       json.RawMessage `json:"payload"`
		Checksum      string          `json:"checksum"`
	}{SchemaVersion: schema, Payload: json.RawMessage(payload), Checksum: "bundle-presence"})
	if err != nil {
		return fmt.Errorf("build bundle presence view: %w", err)
	}
	return validate(envelope)
}
