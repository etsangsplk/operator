/*
Copyright The KubeVault Authors.

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

package v1alpha1

import (
	core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kmapi "kmodules.xyz/client-go/api/v1"
	appcat "kmodules.xyz/custom-resources/apis/appcatalog/v1alpha1"
)

const (
	ResourceKindMySQLRole = "MySQLRole"
	ResourceMySQLRole     = "mysqlrole"
	ResourceMySQLRoles    = "mysqlroles"
)

// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=mysqlroles,singular=mysqlrole,categories={vault,appscode,all}
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Status",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
type MySQLRole struct {
	metav1.TypeMeta   `json:",inline,omitempty"`
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`
	Spec              MySQLRoleSpec   `json:"spec,omitempty" protobuf:"bytes,2,opt,name=spec"`
	Status            MySQLRoleStatus `json:"status,omitempty" protobuf:"bytes,3,opt,name=status"`
}

// MySQLRoleSpec contains connection information, mysql role info etc
type MySQLRoleSpec struct {
	// VaultRef is the name of a AppBinding referencing to a Vault Server
	VaultRef core.LocalObjectReference `json:"vaultRef" protobuf:"bytes,1,opt,name=vaultRef"`

	// DatabaseRef specifies the database appbinding reference in any namespace.
	DatabaseRef *appcat.AppReference `json:"databaseRef,omitempty" protobuf:"bytes,2,opt,name=databaseRef"`

	// Specifies the database name under which the role will be created
	DatabaseName string `json:"databaseName,omitempty" protobuf:"bytes,3,opt,name=databaseName"`

	// Specifies the path where secret engine is enabled
	Path string `json:"path,omitempty" protobuf:"bytes,4,opt,name=path"`

	// links:
	// 	- https://www.vaultproject.io/api/secret/databases/index.html
	//	- https://www.vaultproject.io/api/secret/databases/mysql-maria.html

	// Specifies the TTL for the leases associated with this role.
	// Accepts time suffixed strings ("1h") or an integer number of seconds.
	// Defaults to system/engine default TTL time
	DefaultTTL string `json:"defaultTTL,omitempty" protobuf:"bytes,5,opt,name=defaultTTL"`

	// Specifies the maximum TTL for the leases associated with this role.
	// Accepts time suffixed strings ("1h") or an integer number of seconds.
	// Defaults to system/engine default TTL time.
	MaxTTL string `json:"maxTTL,omitempty" protobuf:"bytes,6,opt,name=maxTTL"`

	// https://www.vaultproject.io/api/secret/databases/mysql-maria.html#creation_statements
	// Specifies the database statements executed to create and configure a user.
	CreationStatements []string `json:"creationStatements" protobuf:"bytes,7,rep,name=creationStatements"`

	// https://www.vaultproject.io/api/secret/databases/mysql-maria.html#revocation_statements
	// Specifies the database statements to be executed to revoke a user.
	RevocationStatements []string `json:"revocationStatements,omitempty" protobuf:"bytes,8,rep,name=revocationStatements"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

type MySQLRoleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// Items is a list of MySQLRole objects
	Items []MySQLRole `json:"items,omitempty" protobuf:"bytes,2,rep,name=items"`
}

type MySQLRolePhase string

type MySQLRoleStatus struct {
	Phase MySQLRolePhase `json:"phase,omitempty" protobuf:"bytes,1,opt,name=phase,casttype=MySQLRolePhase"`

	// ObservedGeneration is the most recent generation observed for this MySQLRole. It corresponds to the
	// MySQLRole's generation, which is updated on mutation by the API Server.
	ObservedGeneration int64 `json:"observedGeneration,omitempty" protobuf:"varint,2,opt,name=observedGeneration"`

	// Represents the latest available observations of a MySQLRole current state.
	Conditions []kmapi.Condition `json:"conditions,omitempty" protobuf:"bytes,3,rep,name=conditions"`
}
