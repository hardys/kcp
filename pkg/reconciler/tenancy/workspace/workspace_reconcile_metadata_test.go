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

package workspace

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1alpha1 "github.com/kcp-dev/kcp/pkg/apis/core/v1alpha1"
	tenancyv1beta1 "github.com/kcp-dev/kcp/pkg/apis/tenancy/v1beta1"
)

func TestReconcileMetadata(t *testing.T) {
	date, err := time.Parse(time.RFC3339, "2006-01-02T15:04:05Z")
	require.NoError(t, err)

	for _, testCase := range []struct {
		name       string
		input      *tenancyv1beta1.Workspace
		expected   metav1.ObjectMeta
		wantStatus reconcileStatus
	}{
		{
			name: "removes everything but owner username when ready",
			input: &tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"tenancy.kcp.io/phase": "Ready",
					},
					Annotations: map[string]string{
						"a":                                 "b",
						"experimental.tenancy.kcp.io/owner": `{"username":"user-1","groups":["a","b"],"uid":"123","extra":{"c":["d"]}}`,
					},
				},
				Status: tenancyv1beta1.WorkspaceStatus{
					Phase: corev1alpha1.LogicalClusterPhaseReady,
				},
			},
			expected: metav1.ObjectMeta{
				Labels: map[string]string{
					"tenancy.kcp.io/phase": "Ready",
				},
				Annotations: map[string]string{
					"a":                                 "b",
					"experimental.tenancy.kcp.io/owner": `{"username":"user-1"}`,
				},
			},
			wantStatus: reconcileStatusStopAndRequeue,
		},
		{
			name: "shows phase Deleting when deletion timestamp is set",
			input: &tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					DeletionTimestamp: &metav1.Time{Time: date},
				},
				Status: tenancyv1beta1.WorkspaceStatus{
					Phase: corev1alpha1.LogicalClusterPhaseReady,
				},
			},
			expected: metav1.ObjectMeta{
				DeletionTimestamp: &metav1.Time{Time: date},
				Labels: map[string]string{
					"tenancy.kcp.io/phase": "Deleting",
				},
			},
			wantStatus: reconcileStatusStopAndRequeue,
		},
		{
			name: "delete invalid owner annotation when ready",
			input: &tenancyv1beta1.Workspace{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"tenancy.kcp.io/phase": "Ready",
					},
					Annotations: map[string]string{
						"a":                                 "b",
						"experimental.tenancy.kcp.io/owner": `{"username":}`,
					},
				},
				Status: tenancyv1beta1.WorkspaceStatus{
					Phase: corev1alpha1.LogicalClusterPhaseReady,
				},
			},
			expected: metav1.ObjectMeta{
				Labels: map[string]string{
					"tenancy.kcp.io/phase": "Ready",
				},
				Annotations: map[string]string{
					"a": "b",
				},
			},
			wantStatus: reconcileStatusStopAndRequeue,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			reconciler := metaDataReconciler{}
			status, err := reconciler.reconcile(context.Background(), testCase.input)

			require.NoError(t, err)
			require.Equal(t, testCase.wantStatus, status)

			if diff := cmp.Diff(testCase.input.ObjectMeta, testCase.expected); diff != "" {
				t.Errorf("invalid output after reconciling metadata: %v", diff)
			}
		})
	}
}
