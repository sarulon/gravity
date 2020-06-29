/*
Copyright 2018 Gravitational, Inc.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package cli

import (
	"context"
	"encoding/json"
	"fmt"
	_ "net/http/pprof"
	"strings"

	"github.com/gravitational/gravity/lib/app/docker"
	appservice "github.com/gravitational/gravity/lib/app/service"
	"github.com/gravitational/gravity/lib/constants"
	"github.com/gravitational/gravity/lib/defaults"
	"github.com/gravitational/gravity/lib/install"
	"github.com/gravitational/gravity/lib/localenv"
	"github.com/gravitational/gravity/lib/ops"
	"github.com/gravitational/gravity/lib/pack"
	"github.com/gravitational/gravity/lib/pack/encryptedpack"
	"github.com/gravitational/gravity/lib/state"
	libstatus "github.com/gravitational/gravity/lib/status"
	"github.com/gravitational/gravity/lib/storage"
	"github.com/gravitational/gravity/lib/users"
	"github.com/gravitational/gravity/lib/utils"
	"github.com/gravitational/gravity/tool/common"

	"github.com/gravitational/license"
	"github.com/gravitational/trace"
)

func selectNetworkInterface() (string, error) {
	for {
		addr, autoselected, err := selectInterface()
		if err != nil {
			return "", trace.Wrap(err)
		}
		if autoselected {
			return addr, nil
		}
		confirmed, err := confirmWithTitle(fmt.Sprintf(
			"\nConfirm the selected interface [%v]", addr))
		if err != nil {
			return "", trace.Wrap(err)
		}
		if !confirmed {
			continue
		}
		return addr, nil
	}
}

func mustJSON(i interface{}) string {
	bytes, err := json.Marshal(i)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}

func appPackage(env *localenv.LocalEnvironment) error {
	apps, err := env.AppServiceLocal(localenv.AppConfig{})
	if err != nil {
		return trace.Wrap(err)
	}

	appPackage, err := install.GetAppPackage(apps)
	if err != nil {
		return trace.Wrap(err)
	}

	fmt.Printf("%v", appPackage)
	return nil
}

func uploadUpdate(ctx context.Context, env *localenv.LocalEnvironment, opsURL string) error {
	// create local environment with gravity state dir because the environment
	// provided above has upgrade tarball as a state dir
	localStateDir, err := localenv.LocalGravityDir()
	if err != nil {
		return trace.Wrap(err)
	}

	defaultEnv, err := localenv.New(localStateDir)
	if err != nil {
		return trace.Wrap(err)
	}

	clusterOperator, err := defaultEnv.SiteOperator()
	if err != nil {
		return trace.Wrap(err, "unable to access cluster.\n"+
			"Use 'gravity status' to check the cluster state and make sure "+
			"that the cluster DNS is working properly.")
	}

	cluster, err := clusterOperator.GetLocalSite(context.TODO())
	if err != nil {
		return trace.Wrap(err)
	}

	if cluster.State == ops.SiteStateDegraded {
		return trace.BadParameter("The cluster is in degraded state so " +
			"uploading new applications is prohibited. Please check " +
			"gravity status output and correct the situation before " +
			"attempting again.")
	}

	var tarballPackages pack.PackageService = env.Packages
	if cluster.License != nil {
		parsed, err := license.ParseLicense(cluster.License.Raw)
		if err != nil {
			return trace.Wrap(err)
		}

		encryptionKey := parsed.GetPayload().EncryptionKey
		if len(encryptionKey) != 0 {
			tarballPackages = encryptedpack.New(tarballPackages, string(encryptionKey))
		}
	}

	clusterPackages, err := defaultEnv.ClusterPackages()
	if err != nil {
		return trace.Wrap(err)
	}

	clusterApps, err := defaultEnv.AppServiceCluster()
	if err != nil {
		return trace.Wrap(err)
	}

	tarballApps, err := env.AppServiceLocal(localenv.AppConfig{
		Packages: tarballPackages,
	})
	if err != nil {
		return trace.Wrap(err)
	}

	appPackage, err := install.GetAppPackage(tarballApps)
	if err != nil {
		return trace.Wrap(err)
	}

	env.PrintStep("Importing application %v v%v", appPackage.Name, appPackage.Version)
	_, err = appservice.PullApp(appservice.AppPullRequest{
		SrcPack: tarballPackages,
		SrcApp:  tarballApps,
		DstPack: clusterPackages,
		DstApp:  clusterApps,
		Package: *appPackage,
	})
	if err != nil {
		if !trace.IsAlreadyExists(err) {
			return trace.Wrap(err)
		}
		env.PrintStep("Application already exists in local cluster")
	}

	var registries []string
	err = utils.Retry(defaults.RetryInterval, defaults.RetryLessAttempts, func() error {
		registries, err = getRegistries(ctx, defaultEnv, cluster.ClusterState.Servers)
		return trace.Wrap(err)
	})
	if err != nil {
		return trace.Wrap(err)
	}

	stateDir, err := state.GetStateDir()
	if err != nil {
		return trace.Wrap(err)
	}

	for _, registry := range registries {
		env.PrintStep("Synchronizing application with Docker registry %v",
			registry)

		imageService, err := docker.NewImageService(docker.RegistryConnectionRequest{
			RegistryAddress: registry,
			CertName:        constants.DockerRegistry,
			CACertPath:      state.Secret(stateDir, defaults.RootCertFilename),
			ClientCertPath:  state.Secret(stateDir, "kubelet.cert"),
			ClientKeyPath:   state.Secret(stateDir, "kubelet.key"),
		})
		if err != nil {
			return trace.Wrap(err)
		}
		err = appservice.SyncApp(ctx, appservice.SyncRequest{
			PackService:  tarballPackages,
			AppService:   tarballApps,
			ImageService: imageService,
			Package:      *appPackage,
		})
		if err != nil {
			return trace.Wrap(err)
		}
	}

	// Uploading new blobs to the cluster is known to cause stress on disk
	// which can lead to the cluster's health checker experiencing momentary
	// blips and potentially moving the cluster to degraded state, especially
	// when running on a hardware with sub-par I/O performance.
	//
	// To accommodate this behavior and make sure upgrade (which normally
	// follows upload right away) does not fail to launch due to the degraded
	// state, give the cluster a few minutes to settle.
	//
	// See https://github.com/gravitational/gravity/issues/1659 for more info.
	env.PrintStep("Verifying cluster health")
	ctx, cancel := context.WithTimeout(ctx, defaults.NodeStatusTimeout)
	defer cancel()
	err = libstatus.WaitCluster(ctx, clusterOperator)
	if err != nil {
		return trace.Wrap(err)
	}

	env.PrintStep("Application has been uploaded")
	return nil
}

// getRegistries returns a list of registry addresses in the cluster
func getRegistries(ctx context.Context, env *localenv.LocalEnvironment, servers []storage.Server) ([]string, error) {
	// in planets before certain version registry was running only on active master
	version, err := planetVersion(env)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	if version.LessThan(*constants.PlanetMultiRegistryVersion) {
		return []string{constants.DockerRegistry}, nil
	}
	// otherwise return registry addresses on all masters
	ips, err := getMasterNodes(ctx, servers)
	if err != nil {
		return nil, trace.Wrap(err)
	}
	registries := make([]string, 0, len(ips))
	for _, ip := range ips {
		registries = append(registries, defaults.DockerRegistryAddr(ip))
	}
	return registries, nil
}

// connectToOpsCenter
func connectToOpsCenter(env *localenv.LocalEnvironment, opsCenterURL, username, password string) (err error) {
	if username == "" || password == "" {
		username, password, err = common.ReadUserPass()
		if err != nil {
			return trace.Wrap(err)
		}
	}
	entry, err := env.Creds.UpsertLoginEntry(
		users.LoginEntry{
			OpsCenterURL: opsCenterURL,
			Email:        username,
			Password:     password})
	if err != nil {
		return trace.Wrap(err)
	}
	fmt.Printf("\n\nconnected to %v\n", *entry)
	return nil
}

// disconnectFromOpsCenter
func disconnectFromOpsCenter(env *localenv.LocalEnvironment, opsCenterURL string) error {
	err := env.Creds.DeleteLoginEntry(opsCenterURL)
	if err != nil && !trace.IsNotFound(err) {
		return trace.Wrap(err)
	}
	fmt.Printf("disconnected from %v", opsCenterURL)
	return nil
}

func listOpsCenters(env *localenv.LocalEnvironment) error {
	entries, err := env.Creds.GetLoginEntries()
	if err != nil {
		return trace.Wrap(err)
	}
	common.PrintHeader("logins")
	for _, entry := range entries {
		fmt.Printf("* %v %v\n", entry.OpsCenterURL, entry.Email)
	}
	fmt.Printf("\n")
	return nil
}

type envvars map[string]string

func newEnvironSource(env []string) (result envvars) {
	result = make(map[string]string)
	for _, variable := range env {
		keyvalue := strings.Split(variable, "=")
		if len(keyvalue) == 2 {
			key, value := keyvalue[0], keyvalue[1]
			result[key] = value
		}
	}
	return result
}

func (r envvars) GetEnv(name string) string {
	return r[name]
}
