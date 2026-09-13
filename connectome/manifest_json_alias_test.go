package connectome

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDecodeManifestRejectsUnicodeCaseAliasDuplicate(t *testing.T) {
	manifest, _ := happyFixture(t, []int{11})
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := strings.Replace(string(data), "{", `{"ſchema_version":"`+ManifestSchemaVersion+`",`, 1)
	_, err = DecodeManifest(strings.NewReader(duplicate))
	if err == nil || !strings.Contains(err.Error(), "duplicate JSON key") {
		t.Fatalf("DecodeManifest error = %v; want duplicate key rejection", err)
	}
}
