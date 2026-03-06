package heronodegroup

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/crossplane/provider-template/apis/compute/v1alpha1"
)

func TestCreate(t *testing.T) {
	type args struct {
		mg resource.Managed
	}
	type want struct {
		err error
	}

	cases := map[string]struct {
		args args
		want want
	}{
		"Success": {
			args: args{
				mg: &v1alpha1.HeroNodeGroup{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-node-group",
					},
					Spec: v1alpha1.HeroNodeGroupSpec{
						ForProvider: v1alpha1.HeroNodeGroupParameters{
							ClusterName: "test-cluster",
							CustomAMI:   "ami-123",
						},
					},
				},
			},
			want: want{
				err: nil,
			},
		},
	}

	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			scheme := runtime.NewScheme()
			v1alpha1.SchemeBuilder.AddToScheme(scheme)
			// Add Karpenter schemes if available, or just generic

			e := &external{kube: fake.NewClientBuilder().WithScheme(scheme).Build()}
			_, err := e.Create(context.Background(), tc.args.mg)

			if diff := cmp.Diff(tc.want.err, err); diff != "" {
				t.Errorf("Create(...): -want error, +got error:\n%s", diff)
			}
		})
	}
}
