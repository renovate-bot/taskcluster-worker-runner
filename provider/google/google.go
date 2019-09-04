package google

import (
	"fmt"
	"strings"

	"github.com/taskcluster/taskcluster-worker-runner/cfg"
	"github.com/taskcluster/taskcluster-worker-runner/protocol"
	"github.com/taskcluster/taskcluster-worker-runner/provider/provider"
	"github.com/taskcluster/taskcluster-worker-runner/run"
	"github.com/taskcluster/taskcluster-worker-runner/tc"
	tcclient "github.com/taskcluster/taskcluster/clients/client-go/v16"
	"github.com/taskcluster/taskcluster/clients/client-go/v16/tcworkermanager"
)

type GoogleProvider struct {
	runnercfg                  *cfg.RunnerConfig
	workerManagerClientFactory tc.WorkerManagerClientFactory
	metadataService            MetadataService
	proto                      *protocol.Protocol
}

func (p *GoogleProvider) ConfigureRun(state *run.State) error {
	workerID, err := p.metadataService.queryMetadata("/instance/id")
	if err != nil {
		return fmt.Errorf("Could not query metadata: %v", err)
	}

	userData, err := p.metadataService.queryUserData()
	if err != nil {
		return fmt.Errorf("Could not query user data: %v", err)
	}

	state.RootURL = userData.RootURL

	// the worker identity
	proofPath := fmt.Sprintf("/instance/service-accounts/default/identity?audience=%s&format=full", userData.RootURL)
	proofToken, err := p.metadataService.queryMetadata(proofPath)
	if err != nil {
		return err
	}

	// We need a worker manager client for fetching taskcluster credentials.
	// Ensure auth is disabled in client, since we don't have credentials yet.
	wm, err := p.workerManagerClientFactory(state.RootURL, nil)
	if err != nil {
		return fmt.Errorf("Could not create worker manager client: %v", err)
	}

	workerIdentityProofMap := map[string]interface{}{"token": interface{}(proofToken)}

	err = provider.RegisterWorker(state, wm, userData.WorkerPoolID, userData.ProviderID, userData.WorkerGroup, workerID, workerIdentityProofMap)
	if err != nil {
		return err
	}

	providerMetadata := map[string]string{
		"instance-id": workerID,
	}
	for _, f := range []struct {
		name string
		path string
	}{
		{"project-id", "/project/project-id"},
		{"image", "/instance/image"},
		{"instance-type", "/instance/machine-type"},
		{"zone", "/instance/zone"},
		{"public-hostname", "/instance/hostname"},
		{"public-ipv4", "/instance/network-interfaces/0/access-configs/0/external-ip"},
		{"local-ipv4", "/instance/network-interfaces/0/ip"},
	} {
		value, err := p.metadataService.queryMetadata(f.path)
		if err != nil {
			return fmt.Errorf("Error querying GCE metadata %v: %v", f.path, err)
		}
		providerMetadata[f.name] = value
	}

	// post process the `zone` value into just the zone name, and the region name
	// region is available as availability-zone minus the final letter
	zone := providerMetadata["zone"]
	zone = zone[strings.LastIndexByte(zone, byte('/'))+1:]
	providerMetadata["zone"] = zone
	providerMetadata["region"] = zone[:len(zone)-2]

	state.WorkerLocation = map[string]string{
		"cloud":  "google",
		"region": providerMetadata["region"],
		"zone":   providerMetadata["zone"],
	}

	state.ProviderMetadata = providerMetadata

	state.WorkerConfig = state.WorkerConfig.Merge(userData.WorkerConfig)

	return nil
}

func (p *GoogleProvider) UseCachedRun(run *run.State) error {
	return nil
}

func (p *GoogleProvider) SetProtocol(proto *protocol.Protocol) {
	p.proto = proto
}

func (p *GoogleProvider) WorkerStarted() error {
	return nil
}

func (p *GoogleProvider) WorkerFinished() error {
	return nil
}

func clientFactory(rootURL string, credentials *tcclient.Credentials) (tc.WorkerManager, error) {
	prov := tcworkermanager.New(credentials, rootURL)
	return prov, nil
}

func New(runnercfg *cfg.RunnerConfig) (provider.Provider, error) {
	return new(runnercfg, nil, nil)
}

func Usage() string {
	return `
The providerType "google" is intended for workers provisioned with worker-manager
providers using providerType "google".  It requires

	provider:
		providerType: google

The TASKCLUSTER_WORKER_LOCATION of this provider has the following fields:

- cloud: aws
- region
- zone
`
}

// New takes its dependencies as optional arguments, allowing injection of fake dependencies for testing.
func new(runnercfg *cfg.RunnerConfig, workerManagerClientFactory tc.WorkerManagerClientFactory, metadataService MetadataService) (*GoogleProvider, error) {
	if workerManagerClientFactory == nil {
		workerManagerClientFactory = clientFactory
	}
	if metadataService == nil {
		metadataService = &realMetadataService{}
	}
	return &GoogleProvider{
		runnercfg:                  runnercfg,
		workerManagerClientFactory: workerManagerClientFactory,
		metadataService:            metadataService,
		proto:                      nil,
	}, nil
}
