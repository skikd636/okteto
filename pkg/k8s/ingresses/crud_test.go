// Copyright 2022 The Okteto Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ingresses

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/okteto/okteto/pkg/model"
	networkingv1 "k8s.io/api/networking/v1"
	networkingv1beta1 "k8s.io/api/networking/v1beta1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	k8sTesting "k8s.io/client-go/testing"
)

func TestCreate(t *testing.T) {
	ctx := context.Background()
	i := &Ingress{
		V1: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fake",
				Namespace: "test",
			},
		},
	}

	clientset := fake.NewSimpleClientset()
	iClient := Client{
		c:    clientset,
		isV1: true,
	}
	err := iClient.Create(ctx, i)
	if err != nil {
		t.Fatal(err)
	}
	retrieved, err := clientset.NetworkingV1().Ingresses(i.V1.Namespace).Get(ctx, i.V1.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(retrieved, i.V1) {
		t.Fatalf("Didn't created correctly")
	}
}

func TestUpdate(t *testing.T) {
	ctx := context.Background()
	labels := map[string]string{"key": "value"}
	i := &Ingress{
		V1: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fake",
				Namespace: "test",
				Labels:    labels,
			},
		},
	}

	clientset := fake.NewSimpleClientset(i.V1)
	iClient := Client{
		c:    clientset,
		isV1: true,
	}

	updatedLabels := map[string]string{"key": "value", "key2": "value2"}
	updatedIngress := &Ingress{
		V1: &networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "fake",
				Namespace: "test",
				Labels:    updatedLabels,
			},
		},
	}
	err := iClient.Update(ctx, updatedIngress)
	if err != nil {
		t.Fatal(err)
	}

	retrieved, err := clientset.NetworkingV1().Ingresses(i.V1.Namespace).Get(ctx, i.V1.Name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(retrieved, updatedIngress.V1) {
		t.Fatalf("Didn't updated correctly")
	}
}

func TestList(t *testing.T) {
	ctx := context.Background()
	i := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fake",
			Namespace: "test",
		},
	}

	clientset := fake.NewSimpleClientset(i)
	iClient := Client{
		c:    clientset,
		isV1: true,
	}
	iList, err := iClient.List(ctx, i.Namespace, "")
	if err != nil {
		t.Fatal(err)
	}

	if len(iList) != 1 {
		t.Fatal(fmt.Errorf("Expected 1 ingress, found %d", len(iList)))
	}

}

func TestDestroy(t *testing.T) {
	var tests = []struct {
		name      string
		iName     string
		namespace string
		i         *networkingv1.Ingress
	}{
		{
			name:      "existent-ingress",
			iName:     "ingress-test",
			namespace: "test",
			i: &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "ingress-test",
					Namespace: "test",
				},
			},
		},
		{
			name:      "ingress-not-found",
			iName:     "ingress-test",
			namespace: "test",
			i: &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "non-existent-ingress",
					Namespace: "another-space",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			clientset := fake.NewSimpleClientset(tt.i)
			iClient := Client{
				c:    clientset,
				isV1: true,
			}

			err := iClient.Destroy(ctx, tt.iName, tt.namespace)

			if err != nil {
				t.Fatalf("unexpected error '%s'", err)
			}
		})
	}
}

func TestDestroyWithError(t *testing.T) {
	ctx := context.Background()
	ingressName := "ingress-test"
	namespace := "test"

	kubernetesError := "something went wrong in the test"
	clientset := fake.NewSimpleClientset()
	clientset.Fake.PrependReactor("delete", "ingresses", func(action k8sTesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New(kubernetesError)
	})
	iClient := Client{
		c:    clientset,
		isV1: true,
	}

	err := iClient.Destroy(ctx, ingressName, namespace)

	if err == nil {
		t.Fatal("an error was expected but no error was returned")
	}
	if !strings.Contains(err.Error(), kubernetesError) {
		t.Fatalf("Got '%s' error but expected '%s'", err.Error(), kubernetesError)
	}
}

func Test_applyDivertChanges(t *testing.T) {
	var tests = []struct {
		name     string
		iClient  *Client
		ingress  *Ingress
		old      *networkingv1.Ingress
		expected map[string]string
	}{
		{
			name:    "v1-no-divert",
			iClient: &Client{isV1: true},
			ingress: &Ingress{
				V1: &networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{"key1": "value1"},
					},
				},
			},
			old: &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"key2": "value2"},
				},
			},
			expected: map[string]string{"key1": "value1"},
		},
		{
			name:    "v1-divert",
			iClient: &Client{isV1: true},
			ingress: &Ingress{
				V1: &networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{"key1": "value1"},
					},
				},
			},
			old: &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"key2": "value2",
						model.OktetoDivertIngressInjectionAnnotation:    "cindy",
						model.OktetoNginxConfigurationSnippetAnnotation: "start-divert-end",
					},
				},
			},
			expected: map[string]string{
				"key1": "value1",
				model.OktetoDivertIngressInjectionAnnotation:    "cindy",
				model.OktetoNginxConfigurationSnippetAnnotation: "start-divert-end",
			},
		},
		{
			name:    "v1beta1-no-divert",
			iClient: &Client{isV1: false},
			ingress: &Ingress{
				V1Beta1: &networkingv1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{"key1": "value1"},
					},
				},
			},
			old: &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{"key2": "value2"},
				},
			},
			expected: map[string]string{"key1": "value1"},
		},
		{
			name:    "v1beta1-divert",
			iClient: &Client{isV1: false},
			ingress: &Ingress{
				V1Beta1: &networkingv1beta1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{"key1": "value1"},
					},
				},
			},
			old: &networkingv1.Ingress{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"key2": "value2",
						model.OktetoDivertIngressInjectionAnnotation:    "cindy",
						model.OktetoNginxConfigurationSnippetAnnotation: "start-divert-end",
					},
				},
			},
			expected: map[string]string{
				"key1": "value1",
				model.OktetoDivertIngressInjectionAnnotation:    "cindy",
				model.OktetoNginxConfigurationSnippetAnnotation: "start-divert-end",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.iClient.applyDivertChanges(tt.ingress, tt.old.GetObjectMeta())
			if !reflect.DeepEqual(tt.ingress.GetAnnotations(), tt.expected) {
				t.Fatalf("Didn't updated annotations correctly")
			}
		})
	}
}
