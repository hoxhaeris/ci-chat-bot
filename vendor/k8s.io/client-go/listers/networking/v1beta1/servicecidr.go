/*
Copyright The Kubernetes Authors.

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

// Code generated by lister-gen. DO NOT EDIT.

package v1beta1

import (
	v1beta1 "k8s.io/api/networking/v1beta1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/listers"
	"k8s.io/client-go/tools/cache"
)

// ServiceCIDRLister helps list ServiceCIDRs.
// All objects returned here must be treated as read-only.
type ServiceCIDRLister interface {
	// List lists all ServiceCIDRs in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1beta1.ServiceCIDR, err error)
	// Get retrieves the ServiceCIDR from the index for a given name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1beta1.ServiceCIDR, error)
	ServiceCIDRListerExpansion
}

// serviceCIDRLister implements the ServiceCIDRLister interface.
type serviceCIDRLister struct {
	listers.ResourceIndexer[*v1beta1.ServiceCIDR]
}

// NewServiceCIDRLister returns a new ServiceCIDRLister.
func NewServiceCIDRLister(indexer cache.Indexer) ServiceCIDRLister {
	return &serviceCIDRLister{listers.New[*v1beta1.ServiceCIDR](indexer, v1beta1.Resource("servicecidr"))}
}