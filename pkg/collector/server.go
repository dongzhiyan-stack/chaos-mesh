// Copyright 2020 Chaos Mesh Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// See the License for the specific language governing permissions and
// limitations under the License.

package collector

import (
	"os"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/chaos-mesh/chaos-mesh/api/v1alpha1"
	"github.com/chaos-mesh/chaos-mesh/pkg/clientpool"
	"github.com/chaos-mesh/chaos-mesh/pkg/config"
	"github.com/chaos-mesh/chaos-mesh/pkg/core"
)

var (
	scheme = runtime.NewScheme()
	log    = ctrl.Log.WithName("collector")
)

func init() {
	_ = clientgoscheme.AddToScheme(scheme)

	_ = v1alpha1.AddToScheme(scheme)
}

// Server defines a server to manage collectors.
type Server struct {
	Manager ctrl.Manager
}

// NewServer returns a CollectorServer and Client.
func NewServer(
	conf *config.ChaosDashboardConfig,
	archive core.ExperimentStore,
	event core.EventStore,
) (*Server, client.Client, client.Reader) {
	s := &Server{}

	// namespace scoped
	options := ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: conf.MetricAddress,
		LeaderElection:     conf.EnableLeaderElection,
		Port:               9443,
	}
	if conf.ClusterScoped {
		log.Info("Chaos controller manager is running in cluster scoped mode.")
	} else {
		log.Info("Chaos controller manager is running in namespace scoped mode.", "targetNamespace", conf.TargetNamespace)
		options.Namespace = conf.TargetNamespace
	}

	var err error

	cfg := ctrl.GetConfigOrDie()
	clientpool.K8sClients, err = clientpool.New(cfg, scheme, 100)
	if err != nil {
		// this should never happen
		log.Error(err, "fail to create client pool")
		os.Exit(1)
	}

	s.Manager, err = ctrl.NewManager(cfg, options)
	if err != nil {
		log.Error(err, "unable to start collector")
		os.Exit(1)
	}

	for kind, chaosKind := range v1alpha1.AllKinds() {
		if err = (&ChaosCollector{
			Client:  s.Manager.GetClient(),
			Log:     ctrl.Log.WithName("collector").WithName(kind),
			archive: archive,
			event:   event,
		}).Setup(s.Manager, chaosKind.Chaos); err != nil {
			log.Error(err, "unable to create collector", "collector", kind)
			os.Exit(1)
		}
	}

	return s, s.Manager.GetClient(), s.Manager.GetAPIReader()
}

// Register starts collectors manager.
func Register(s *Server, controllerRuntimeStopCh <-chan struct{}) {
	go func() {
		log.Info("Starting collector")
		if err := s.Manager.Start(controllerRuntimeStopCh); err != nil {
			log.Error(err, "could not start collector")
			os.Exit(1)
		}
	}()
}
