package learning_test

import (
	"testing"

	"github.com/TimLai666/coimnet/learning"
)

func TestTrainerRejectsDeclaredNodesBeforeTopologyAllocation(t *testing.T) {
	network, parameters := network(t)
	config := network.Config()
	config.Dynamics.Nodes = int(^uint(0) >> 1)
	config.Dynamics.Workers = 2
	options := learning.DefaultOptions()
	for _, name := range []string{"new", "restore"} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if value := recover(); value != nil {
					t.Fatalf("untrusted node declaration caused topology allocation panic: %v", value)
				}
			}()
			var err error
			if name == "new" {
				_, err = learning.NewTrainer(config, parameters, options)
			} else {
				_, err = learning.RestoreTrainer(learning.TrainingSnapshot{
					SchemaVersion: "coimnet-episode-training/v1", Config: config,
					Parameters: parameters, Options: options,
				})
			}
			if err == nil {
				t.Fatal("malformed model was accepted")
			}
		})
	}
}
