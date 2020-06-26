package clusterconfig

import (
	"fmt"

	"github.com/gravitational/gravity/lib/loc"
	"github.com/gravitational/gravity/lib/storage"
	"github.com/gravitational/gravity/lib/update"
	"github.com/gravitational/gravity/lib/update/clusterconfig/phases"
	"github.com/gravitational/gravity/lib/update/internal/rollingupdate"
	libphase "github.com/gravitational/gravity/lib/update/internal/rollingupdate/phases"

	v1 "k8s.io/api/core/v1"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
)

func newBuilder(app loc.Locator, services []v1.Service) builder {
	suffix := utilrand.String(4)
	serviceName := fmt.Sprintf("kube-dns-%v", suffix)
	workerServiceName := fmt.Sprintf("kube-dns-worker-%v", suffix)
	return builder{
		Builder: rollingupdate.Builder{
			App: app,
			CustomUpdate: &update.Phase{
				ID:          "services",
				Executor:    libphase.Custom,
				Description: "Reset services",
				Data: &storage.OperationPhaseData{
					Update: &storage.UpdateOperationData{
						ClusterConfig: &storage.ClusterConfigData{
							DNSServiceName:       serviceName,
							DNSWorkerServiceName: workerServiceName,
							Services:             services,
						},
					},
				},
			},
		},
	}
}

func (r builder) fini(desc string) *update.Phase {
	return &update.Phase{
		ID:          "fini",
		Executor:    phases.FiniPhase,
		Description: desc,
		Data: &storage.OperationPhaseData{
			Update: &storage.UpdateOperationData{
				ClusterConfig: &storage.ClusterConfigData{
					DNSServiceName:       r.Builder.CustomUpdate.Data.Update.ClusterConfig.DNSServiceName,
					DNSWorkerServiceName: r.Builder.CustomUpdate.Data.Update.ClusterConfig.DNSWorkerServiceName,
				},
			},
		},
	}
}

type builder struct {
	rollingupdate.Builder
}
