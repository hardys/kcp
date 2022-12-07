/*
Copyright 2022 The KCP Authors.

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

package options

import (
	"github.com/spf13/pflag"

	"k8s.io/component-base/config"
	"k8s.io/component-base/logs"
)

type Options struct {
	QPS        float32
	Burst      int
	NumWorkspaces int
	Timeout    int
	Kubeconfig string
	Context    string
	Logs       *logs.Options
	NoDelete   bool
}

func NewOptions() *Options {
	// Default to -v=2
	logs := logs.NewOptions()
	logs.Config.Verbosity = config.VerbosityLevel(2)

	return &Options{
		Timeout:    60,
		NumWorkspaces: 100,
		QPS:        -1,
		Burst:      -1,
		Kubeconfig: ".kcp/admin.kubeconfig",
		Context:    "base",
		Logs:       logs,
		NoDelete:   false,
	}
}

func (options *Options) AddFlags(fs *pflag.FlagSet) {
	fs.Float32Var(&options.QPS, "qps", options.QPS, "QPS to use when talking to API servers.")
	fs.IntVar(&options.Burst, "burst", options.Burst, "Burst to use when talking to API servers.")
	fs.IntVar(&options.NumWorkspaces, "workspaces", options.NumWorkspaces, "Number of workspaces for the test to create.")
	fs.IntVar(&options.Timeout, "timeout", options.Timeout, "Timeout for waits during the test (seconds).")
	fs.StringVar(&options.Kubeconfig, "kubeconfig", options.Kubeconfig, "Kubeconfig file.")
	fs.StringVar(&options.Context, "context", options.Context, "Context to use in the Kubeconfig file.")
	fs.BoolVar(&options.NoDelete, "no-delete", options.NoDelete, "Do not delete created resources.")
	options.Logs.AddFlags(fs)
}

func (options *Options) Complete() error {
	return nil
}

func (options *Options) Validate() error {
	return nil
}
