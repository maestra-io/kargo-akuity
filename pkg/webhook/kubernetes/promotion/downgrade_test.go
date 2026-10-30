package promotion

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	kargoapi "github.com/akuity/kargo/api/v1alpha1"
	k8sevent "github.com/akuity/kargo/pkg/event/kubernetes"
	fakeevent "github.com/akuity/kargo/pkg/kubernetes/event/fake"
)

func Test_findDowngrades(t *testing.T) {
	const testRepo = "example.com/svc-manifests"
	testCases := []struct {
		name     string
		current  *kargoapi.FreightReference
		target   *kargoapi.Freight
		expected []string
	}{
		{
			name:   "no current Freight",
			target: &kargoapi.Freight{Images: []kargoapi.Image{{RepoURL: testRepo, Tag: "1.0.0"}}},
		},
		{
			name:    "image upgrade",
			current: &kargoapi.FreightReference{Images: []kargoapi.Image{{RepoURL: testRepo, Tag: "1.1.33"}}},
			target:  &kargoapi.Freight{Images: []kargoapi.Image{{RepoURL: testRepo, Tag: "1.1.34"}}},
		},
		{
			name:    "same image tag",
			current: &kargoapi.FreightReference{Images: []kargoapi.Image{{RepoURL: testRepo, Tag: "1.1.34"}}},
			target:  &kargoapi.Freight{Images: []kargoapi.Image{{RepoURL: testRepo, Tag: "1.1.34"}}},
		},
		{
			name:     "image downgrade",
			current:  &kargoapi.FreightReference{Images: []kargoapi.Image{{RepoURL: testRepo, Tag: "1.1.34"}}},
			target:   &kargoapi.Freight{Images: []kargoapi.Image{{RepoURL: testRepo, Tag: "1.1.33"}}},
			expected: []string{"image " + testRepo + " 1.1.34 -> 1.1.33"},
		},
		{
			name:     "image downgrade compares numerically, not lexically",
			current:  &kargoapi.FreightReference{Images: []kargoapi.Image{{RepoURL: testRepo, Tag: "1.0.10"}}},
			target:   &kargoapi.Freight{Images: []kargoapi.Image{{RepoURL: testRepo, Tag: "1.0.9"}}},
			expected: []string{"image " + testRepo + " 1.0.10 -> 1.0.9"},
		},
		{
			name:     "v-prefixed image tags",
			current:  &kargoapi.FreightReference{Images: []kargoapi.Image{{RepoURL: testRepo, Tag: "v2.0.0"}}},
			target:   &kargoapi.Freight{Images: []kargoapi.Image{{RepoURL: testRepo, Tag: "v1.9.0"}}},
			expected: []string{"image " + testRepo + " v2.0.0 -> v1.9.0"},
		},
		{
			name:    "non-semver image tag is not judged",
			current: &kargoapi.FreightReference{Images: []kargoapi.Image{{RepoURL: testRepo, Tag: "latest"}}},
			target:  &kargoapi.Freight{Images: []kargoapi.Image{{RepoURL: testRepo, Tag: "1.0.0"}}},
		},
		{
			name: "image from a different repository is not compared",
			current: &kargoapi.FreightReference{
				Images: []kargoapi.Image{{RepoURL: "example.com/other", Tag: "9.0.0"}},
			},
			target: &kargoapi.Freight{Images: []kargoapi.Image{{RepoURL: testRepo, Tag: "1.0.0"}}},
		},
		{
			name: "chart downgrade",
			current: &kargoapi.FreightReference{
				Charts: []kargoapi.Chart{{RepoURL: "oci://example.com/charts", Name: "cdp-release", Version: "4.9220.0"}},
			},
			target: &kargoapi.Freight{
				Charts: []kargoapi.Chart{{RepoURL: "oci://example.com/charts", Name: "cdp-release", Version: "4.9219.0"}},
			},
			expected: []string{"chart oci://example.com/charts/cdp-release 4.9220.0 -> 4.9219.0"},
		},
		{
			name: "chart with a different name is not compared",
			current: &kargoapi.FreightReference{
				Charts: []kargoapi.Chart{{RepoURL: "oci://example.com/charts", Name: "a", Version: "2.0.0"}},
			},
			target: &kargoapi.Freight{
				Charts: []kargoapi.Chart{{RepoURL: "oci://example.com/charts", Name: "b", Version: "1.0.0"}},
			},
		},
		{
			name: "commit tag downgrade",
			current: &kargoapi.FreightReference{
				Commits: []kargoapi.GitCommit{{RepoURL: "https://example.com/repo", Tag: "v1.2.0"}},
			},
			target: &kargoapi.Freight{
				Commits: []kargoapi.GitCommit{{RepoURL: "https://example.com/repo", Tag: "v1.1.0"}},
			},
			expected: []string{"git https://example.com/repo v1.2.0 -> v1.1.0"},
		},
		{
			name: "untagged commits are not judged",
			current: &kargoapi.FreightReference{
				Commits: []kargoapi.GitCommit{{RepoURL: "https://example.com/repo", ID: "bbb"}},
			},
			target: &kargoapi.Freight{
				Commits: []kargoapi.GitCommit{{RepoURL: "https://example.com/repo", ID: "aaa"}},
			},
		},
		{
			name: "generic artifact downgrade",
			current: &kargoapi.FreightReference{
				Artifacts: []kargoapi.ArtifactReference{{SubscriptionName: "bundle", Version: "3.0.0"}},
			},
			target: &kargoapi.Freight{
				Artifacts: []kargoapi.ArtifactReference{{SubscriptionName: "bundle", Version: "2.5.0"}},
			},
			expected: []string{"artifact bundle 3.0.0 -> 2.5.0"},
		},
		{
			name: "one downgraded artifact among upgrades is reported",
			current: &kargoapi.FreightReference{
				Images: []kargoapi.Image{
					{RepoURL: testRepo, Tag: "1.0.0"},
					{RepoURL: "example.com/sidecar", Tag: "2.0.0"},
				},
			},
			target: &kargoapi.Freight{
				Images: []kargoapi.Image{
					{RepoURL: testRepo, Tag: "1.0.1"},
					{RepoURL: "example.com/sidecar", Tag: "1.9.9"},
				},
			},
			expected: []string{"image example.com/sidecar 2.0.0 -> 1.9.9"},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(
				t,
				testCase.expected,
				findDowngrades(testCase.current, testCase.target),
			)
		})
	}
}

func Test_webhook_ValidateCreate_Downgrade(t *testing.T) {
	const testRepo = "example.com/svc-manifests"
	origin := kargoapi.FreightOrigin{
		Kind: kargoapi.FreightOriginKindWarehouse,
		Name: "fake-warehouse",
	}
	freightWithTag := func(name, tag string) *kargoapi.Freight {
		return &kargoapi.Freight{
			ObjectMeta: metav1.ObjectMeta{Name: name},
			Origin:     origin,
			Images:     []kargoapi.Image{{RepoURL: testRepo, Tag: tag}},
		}
	}
	stageOnTag := func(tag string) *kargoapi.Stage {
		stage := &kargoapi.Stage{
			ObjectMeta: metav1.ObjectMeta{Name: "fake-stage"},
			Spec: kargoapi.StageSpec{
				RequestedFreight: []kargoapi.FreightRequest{{
					Origin:  origin,
					Sources: kargoapi.FreightSources{Direct: true},
				}},
			},
		}
		if tag != "" {
			current := &kargoapi.FreightCollection{}
			current.UpdateOrPush(kargoapi.FreightReference{
				Name:   "current",
				Origin: origin,
				Images: []kargoapi.Image{{RepoURL: testRepo, Tag: tag}},
			})
			stage.Status.FreightHistory.Record(current)
		}
		return stage
	}
	newTestWebhook := func(
		stage *kargoapi.Stage,
		freight *kargoapi.Freight,
	) *webhook {
		return &webhook{
			validateProjectFn: func(context.Context, client.Client, client.Object) error {
				return nil
			},
			authorizeFn: func(context.Context, *kargoapi.Promotion, string) error {
				return nil
			},
			admissionRequestFromContextFn: admission.RequestFromContext,
			getStageFn: func(
				context.Context,
				client.Client,
				types.NamespacedName,
			) (*kargoapi.Stage, error) {
				return stage, nil
			},
			getFreightFn: func(
				context.Context,
				client.Client,
				types.NamespacedName,
			) (*kargoapi.Freight, error) {
				return freight, nil
			},
			// Treat every request as coming from the control plane so the
			// cases cover the auto-promotion path too: it gets no exemption.
			isRequestFromKargoControlplaneFn: func(admission.Request) bool {
				return true
			},
		}
	}
	rollbackAnnotation := map[string]string{
		kargoapi.AnnotationKeyRollback: kargoapi.AnnotationValueTrue,
	}

	testCases := []struct {
		name        string
		stage       *kargoapi.Stage
		freight     *kargoapi.Freight
		annotations map[string]string
		assertions  func(*testing.T, error)
	}{
		{
			name:    "Stage has no current Freight",
			stage:   stageOnTag(""),
			freight: freightWithTag("older", "1.1.33"),
			assertions: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name:    "upgrade is allowed",
			stage:   stageOnTag("1.1.33"),
			freight: freightWithTag("newer", "1.1.34"),
			assertions: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name:    "downgrade is denied as Forbidden",
			stage:   stageOnTag("1.1.34"),
			freight: freightWithTag("older", "1.1.33"),
			assertions: func(t *testing.T, err error) {
				require.True(t, apierrors.IsForbidden(err))
				var statusErr *apierrors.StatusError
				require.True(t, errors.As(err, &statusErr))
				require.Contains(
					t,
					statusErr.ErrStatus.Message,
					`promoting Freight "older" would downgrade Stage "fake-stage" `+
						"(image "+testRepo+" 1.1.34 -> 1.1.33)",
				)
				require.Contains(
					t,
					statusErr.ErrStatus.Message,
					kargoapi.AnnotationKeyRollback,
				)
			},
		},
		{
			name:        "downgrade marked as rollback is allowed",
			stage:       stageOnTag("1.1.34"),
			freight:     freightWithTag("older", "1.1.33"),
			annotations: rollbackAnnotation,
			assertions: func(t *testing.T, err error) {
				require.NoError(t, err)
			},
		},
		{
			name:        "rollback annotation other than true does not override",
			stage:       stageOnTag("1.1.34"),
			freight:     freightWithTag("older", "1.1.33"),
			annotations: map[string]string{kargoapi.AnnotationKeyRollback: "yes"},
			assertions: func(t *testing.T, err error) {
				require.True(t, apierrors.IsForbidden(err))
			},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			w := newTestWebhook(testCase.stage, testCase.freight)
			w.sender = k8sevent.NewEventSender(fakeevent.NewEventRecorder(1))
			promo := &kargoapi.Promotion{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "fake-promotion",
					Annotations: testCase.annotations,
				},
				Spec: kargoapi.PromotionSpec{
					Stage:   testCase.stage.Name,
					Freight: testCase.freight.Name,
				},
			}
			ctx := admission.NewContextWithRequest(t.Context(), admission.Request{})
			_, err := w.ValidateCreate(ctx, promo)
			testCase.assertions(t, err)
		})
	}
}
