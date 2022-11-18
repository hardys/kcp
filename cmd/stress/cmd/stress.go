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

package cmd

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/version"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"

	tenancyv1alpha1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1alpha1"
	tenancyv1beta1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1beta1"

	stressoptions "github.com/kcp-dev/kcp/cmd/stress/options"
	kcpclient "github.com/kcp-dev/kcp/pkg/client/clientset/versioned"

	pluginhelpers "github.com/kcp-dev/kcp/pkg/cliplugins/helpers"
)

func NewStressCommand() *cobra.Command {
	options := stressoptions.NewOptions()
	stressCommand := &cobra.Command{
		Use:   "stress",
		Short: "Stress test kcp deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := options.Complete(); err != nil {
				return err
			}

			if err := options.Validate(); err != nil {
				return err
			}

			ctx := context.Background()
			if err := Run(ctx, options); err != nil {
				return err
			}

			return nil
		},
	}

	options.AddFlags(stressCommand.Flags())

	if v := version.Get().String(); len(v) == 0 {
		stressCommand.Version = "<unknown>"
	} else {
		stressCommand.Version = v
	}

	return stressCommand
}

func Run(ctx context.Context, options *stressoptions.Options) error {
	logger := klog.FromContext(ctx)
	logger.Info("stress test")
	err := workspaceCRUD(ctx, options)
	if err != nil {
		logger.Error(err, "Error in workspaceCRUD")
		return err
	}
	logger.Info("done")
	return nil
}

func kcpClient(options *stressoptions.Options) (*kcpclient.Cluster, error) {
	kcpConfigOverrides := &clientcmd.ConfigOverrides{
		CurrentContext: options.Context,
	}
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		&clientcmd.ClientConfigLoadingRules{ExplicitPath: options.Kubeconfig},
		kcpConfigOverrides).ClientConfig()
	if err != nil {
		return nil, err
	}

	config.QPS = options.QPS
	config.Burst = options.Burst

	kcpClusterClient, err := kcpclient.NewClusterForConfig(rest.CopyConfig(config))
	if err != nil {
		return nil, err
	}

	return kcpClusterClient, nil
}

// Create a workspace
func createWorkspace(ctx context.Context, kcpClusterClient *kcpclient.Cluster, parent *tenancyv1beta1.Workspace, wsName string) (*tenancyv1beta1.Workspace, error) {
	logger := klog.FromContext(ctx).WithValues("createWorkspace", wsName)
	// Create the workspace
	_, currentClusterName, err := pluginhelpers.ParseClusterURL(parent.Status.URL)
	if err != nil {
		return nil, fmt.Errorf("current URL %q does not point to cluster workspace", parent.Status.URL)
	}
	ws, err := kcpClusterClient.Cluster(currentClusterName).TenancyV1beta1().Workspaces().Create(ctx, &tenancyv1beta1.Workspace{
		ObjectMeta: metav1.ObjectMeta{
			Name: wsName,
		},
		//Spec: tenancyv1beta1.WorkspaceSpec{
		//	Type: structuredWorkspaceType,
		//},
	}, metav1.CreateOptions{})
	if err != nil {
		logger.Info("Error creating workspace")
		return nil, err
	}
	logger.WithValues("ws", ws.Name).Info("created workspace")

	/* FIXME: Do we want the option to wait for each workspace or only list-wait as below?
	// Wait for it to be ready
	if ws.Status.Phase != tenancyv1alpha1.ClusterWorkspacePhaseReady {
		logger.WithValues("phase", ws.Status.Phase).Info("phase not ready")
		if err := wait.PollImmediate(time.Millisecond*500, time.Second*5, func() (bool, error) {
			ws, err = kcpClusterClient.Cluster(currentClusterName).TenancyV1beta1().Workspaces().Get(ctx, ws.Name, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if ws.Status.Phase == tenancyv1alpha1.ClusterWorkspacePhaseReady {
				logger.WithValues("phase", ws.Status.Phase).Info("phase ready")
				return true, nil
			}
			logger.WithValues("phase", ws.Status.Phase).Info("phase waiting...")
			return false, nil
		}); err != nil {
			logger.Error(err, "Error waiting for status to become ready")
			return nil, err
		}
	}
	*/
	return ws, nil
}

// Wait for workspaces to be ready
func waitForWorkspacesReady(ctx context.Context, kcpClusterClient *kcpclient.Cluster, parent *tenancyv1beta1.Workspace, expectedReady int) error {
	logger := klog.FromContext(ctx).WithValues("wait", "ready")
	// Wait for workspaces to be ready
	_, currentClusterName, err := pluginhelpers.ParseClusterURL(parent.Status.URL)
	if err != nil {
		return fmt.Errorf("current URL %q does not point to cluster workspace", parent.Status.URL)
	}

	// FIXME: we can't select by phase=ready
	// listOptions := metav1.ListOptions{ FieldSelector: "status.phase=Ready", }
	list, err := kcpClusterClient.Cluster(currentClusterName).TenancyV1beta1().Workspaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Info("Error getting workspace list")
		return err
	}

	numReady := 0
	for _, ws := range(list.Items) {
		if ws.Status.Phase == tenancyv1alpha1.ClusterWorkspacePhaseReady {
			logger.WithValues("ws", ws.Name).WithValues("phase", ws.Status.Phase).Info("phase ready")
			numReady += 1
		}
	}

	if numReady != expectedReady {
		logger.WithValues("len", len(list.Items)).Info("not yet deleted")
		if err := wait.PollImmediate(time.Millisecond*500, time.Second*60, func() (bool, error) {
			list, err = kcpClusterClient.Cluster(currentClusterName).TenancyV1beta1().Workspaces().List(ctx, metav1.ListOptions{})
			if err != nil {
				return false, err
			}
			numReady = 0
			for _, ws := range(list.Items) {
				if ws.Status.Phase == tenancyv1alpha1.ClusterWorkspacePhaseReady {
					logger.WithValues("ws", ws.Name, "phase", ws.Status.Phase, "URL", ws.Status.URL).Info("phase ready")
					numReady += 1
				}
			}

			if numReady == expectedReady {
				logger.WithValues("numReady", numReady).Info("all ready")
				return true, nil
			}
			logger.WithValues("numReady", numReady).Info("ready waiting...")
			return false, nil
		}); err != nil {
			logger.Error(err, "Error waiting for ready")
		}
	}
	logger.Info("all ready")
	return nil
}

// Delete a workspace
func deleteWorkspace(ctx context.Context, kcpClusterClient *kcpclient.Cluster, parent *tenancyv1beta1.Workspace, wsName string) error {
	logger := klog.FromContext(ctx).WithValues("deleteWorkspace", wsName)
	// Delete the workspace
	_, currentClusterName, err := pluginhelpers.ParseClusterURL(parent.Status.URL)
	if err != nil {
		return fmt.Errorf("current URL %q does not point to cluster workspace", parent.Status.URL)
	}
	err = kcpClusterClient.Cluster(currentClusterName).TenancyV1beta1().Workspaces().Delete(ctx, wsName, metav1.DeleteOptions{})
	if err != nil {
		logger.Info("Error deleting workspace")
		return err
	}
	logger.WithValues("ws", wsName).Info("deleted workspace")
	return nil
}

// Wait for zero workspaces
func waitWorkspaceDeletion(ctx context.Context, kcpClusterClient *kcpclient.Cluster, parent *tenancyv1beta1.Workspace) error {
	logger := klog.FromContext(ctx).WithValues("wait", "Deleted Workspaces")
	_, currentClusterName, err := pluginhelpers.ParseClusterURL(parent.Status.URL)
	if err != nil {
		return fmt.Errorf("current URL %q does not point to cluster workspace", parent.Status.URL)
	}
	list, err := kcpClusterClient.Cluster(currentClusterName).TenancyV1beta1().Workspaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Info("Error getting workspace list")
		return err
	}

	if len(list.Items) != 0 {
		logger.WithValues("len", len(list.Items)).Info("not yet deleted")
		if err := wait.PollImmediate(time.Millisecond*500, time.Second*60, func() (bool, error) {
			list, err = kcpClusterClient.Cluster(currentClusterName).TenancyV1beta1().Workspaces().List(ctx, metav1.ListOptions{})
			if err != nil {
				return false, err
			}
			if len(list.Items) == 0 {
				logger.Info("all deleted")
				return true, nil
			}
			logger.WithValues("len", len(list.Items)).Info("deleted waiting...")
			return false, nil
		}); err != nil {
			logger.Error(err, "Error waiting for deletion")
		}
	}
	logger.Info("all deleted")
	return nil
}

// Wait for a single workspace to be ready
func waitForWorkspaceReady(ctx context.Context, kcpClusterClient *kcpclient.Cluster, parent *tenancyv1beta1.Workspace, wsName string) (*tenancyv1beta1.Workspace, error) {
	logger := klog.FromContext(ctx).WithValues("wait", "workspace")

	_, currentClusterName, err := pluginhelpers.ParseClusterURL(parent.Status.URL)
	if err != nil {
		return nil, fmt.Errorf("current URL %q does not point to cluster workspace", parent.Status.URL)
	}

	ws, err := kcpClusterClient.Cluster(currentClusterName).TenancyV1beta1().Workspaces().Get(ctx, wsName, metav1.GetOptions{})
	if err != nil {
		logger.Info("Error getting workspace")
		return nil, err
	}

	if ws.Status.Phase != tenancyv1alpha1.ClusterWorkspacePhaseReady {
		if err := wait.PollImmediate(time.Millisecond*500, time.Second*60, func() (bool, error) {
			ws, err = kcpClusterClient.Cluster(currentClusterName).TenancyV1beta1().Workspaces().Get(ctx, wsName, metav1.GetOptions{})
			if err != nil {
				return false, err
			}
			if ws.Status.Phase == tenancyv1alpha1.ClusterWorkspacePhaseReady {
				logger.WithValues("ws", ws.Name).Info("phase ready")
				return true, nil
			}
			logger.WithValues("ws", ws.Name).Info("ready waiting...")
			return false, nil
		}); err != nil {
			logger.Error(err, "Error waiting for ready")
		}
	}

	logger.WithValues("ws", ws.Name).Info("Workspace ready")
	return ws, nil
}

func workspaceCRUD(ctx context.Context, options *stressoptions.Options) error {
	logger := klog.FromContext(ctx).WithValues("test", "workspaceCRUD")
	// Create a KCP client
	kcpClusterClient, err := kcpClient(options)
	if err != nil {
		return err
	}

	// Get home workspace
	home, err := kcpClusterClient.Cluster(tenancyv1alpha1.RootCluster).TenancyV1beta1().Workspaces().Get(ctx, "~", metav1.GetOptions{})
	if err != nil {
		logger.Info("Error getting home workspace")
		return err
	}
	//logger.WithValues("home", home).Info("home workspace")

	// Create test workspace in the home workspace
	wsName := fmt.Sprintf("stress-%d", os.Getpid())
	ws, err := createWorkspace(ctx, kcpClusterClient, home, wsName)
	if err != nil {
		logger.Info("Error creating workspace")
		return err
	}
	logger.WithValues("ws", ws.Name).Info("created workspace")

	// Wait for the test workspace to become ready
	ws, err = waitForWorkspaceReady(ctx, kcpClusterClient, home, wsName)
	if err != nil {
		logger.Info("Error waiting for ready")
		return err
	}

	// Create Workspaces in the test workspace
	createStart := time.Now()
	count := 0
	for count < options.NumWorkspaces {
		count += 1
		twsName := fmt.Sprintf("%s-%d", wsName, count)
		tws, err := createWorkspace(ctx, kcpClusterClient, ws, twsName)
		if err != nil {
			logger.Info("Error creating nested workspace")
			return err
		}
		logger.WithValues("tws", tws.Name).Info("created nested workspace")
	}

	// Wait for all created workspaces to become ready
	err = waitForWorkspacesReady(ctx, kcpClusterClient, ws, options.NumWorkspaces)
	if err != nil {
		logger.Info("Error waiting for ready")
		return err
	}
	createElapsed := time.Since(createStart)

	// Delete workspaces
	deleteStart := time.Now()
	for count > 0 {
		twsName := fmt.Sprintf("%s-%d", wsName, count)
		err := deleteWorkspace(ctx, kcpClusterClient, ws, twsName)
		if err != nil {
			logger.Info("Error deleting nested workspace")
			return err
		}
		count -= 1
	}

	// Confirm deletion
	err = waitWorkspaceDeletion(ctx, kcpClusterClient, ws)
	if err != nil {
		logger.Info("Error waiting for deletion")
		return err
	}
	deleteElapsed := time.Since(deleteStart)

	logger.WithValues("workspaces", options.NumWorkspaces, "created", createElapsed, "deleted", deleteElapsed).Info("completed")
	return nil
}
