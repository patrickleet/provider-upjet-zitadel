// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"sync"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	ujconfig "github.com/crossplane/upjet/v2/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterdomain "github.com/crossplane-contrib/provider-upjet-zitadel/apis/cluster/domain/v1alpha1"
	clusterorg "github.com/crossplane-contrib/provider-upjet-zitadel/apis/cluster/org/v1alpha1"
	clusteruser "github.com/crossplane-contrib/provider-upjet-zitadel/apis/cluster/user/v1alpha1"
	domain "github.com/crossplane-contrib/provider-upjet-zitadel/apis/namespaced/domain/v1alpha1"
	org "github.com/crossplane-contrib/provider-upjet-zitadel/apis/namespaced/org/v1alpha1"
	user "github.com/crossplane-contrib/provider-upjet-zitadel/apis/namespaced/user/v1alpha1"
)

var (
	providersOnce sync.Once
	providers     map[string]*ujconfig.Provider
)

// bothProviders returns the cluster and namespaced provider configurations,
// built once because parsing the schema is expensive.
func bothProviders() map[string]*ujconfig.Provider {
	providersOnce.Do(func() {
		providers = map[string]*ujconfig.Provider{
			"cluster":    GetProvider(),
			"namespaced": GetProviderNamespaced(),
		}
	})
	return providers
}

// assertReference checks a configured reference on both the cluster and the
// namespaced provider.
func assertReference(t *testing.T, resource, field string, want ujconfig.Reference) {
	t.Helper()
	for scope, p := range bothProviders() {
		r, ok := p.Resources[resource]
		if !ok {
			t.Fatalf("%s: resource %s is not generated", scope, resource)
		}
		got, ok := r.References[field]
		if !ok {
			t.Errorf("%s: %s.%s has no reference", scope, resource, field)
			continue
		}
		if got.TerraformName != want.TerraformName || got.RefFieldName != want.RefFieldName || got.SelectorFieldName != want.SelectorFieldName {
			t.Errorf("%s: %s.%s reference = %+v, want %+v", scope, resource, field, got, want)
		}
	}
}

func TestOrgScopedResourcesReferenceOrganization(t *testing.T) {
	for _, resource := range []string{
		"zitadel_action",
		"zitadel_domain",
		"zitadel_domain_claimed_message_text",
		"zitadel_domain_policy",
		"zitadel_init_message_text",
		"zitadel_label_policy",
		"zitadel_lockout_policy",
		"zitadel_login_policy",
		"zitadel_login_texts",
		"zitadel_notification_policy",
		"zitadel_org_idp_apple",
		"zitadel_org_idp_azure_ad",
		"zitadel_org_idp_github",
		"zitadel_org_idp_github_es",
		"zitadel_org_idp_gitlab",
		"zitadel_org_idp_gitlab_self_hosted",
		"zitadel_org_idp_google",
		"zitadel_org_idp_jwt",
		"zitadel_org_idp_ldap",
		"zitadel_org_idp_oauth",
		"zitadel_org_idp_oidc",
		"zitadel_org_idp_saml",
		"zitadel_password_age_policy",
		"zitadel_password_change_message_text",
		"zitadel_password_complexity_policy",
		"zitadel_password_reset_message_text",
		"zitadel_passwordless_registration_message_text",
		"zitadel_privacy_policy",
		"zitadel_trigger_actions",
		"zitadel_verify_email_message_text",
		"zitadel_verify_email_otp_message_text",
		"zitadel_verify_phone_message_text",
		"zitadel_verify_sms_otp_message_text",
	} {
		t.Run(resource, func(t *testing.T) {
			assertReference(t, resource, "org_id", ujconfig.Reference{TerraformName: "zitadel_organization"})
		})
	}
}

// A resource's own identifier and arguments that do not name an owning
// organization must not become references.
func TestIdentityArgumentsAreNotReferences(t *testing.T) {
	for _, tc := range []struct{ resource, field string }{
		{"zitadel_org", "org_id"},
		{"zitadel_organization", "org_id"},
		{"zitadel_webkey", "org_id"},
		{"zitadel_active_webkey", "org_id"},
		{"zitadel_human_user", "user_id"},
		{"zitadel_machine_user", "user_id"},
	} {
		for scope, p := range bothProviders() {
			if ref, ok := p.Resources[tc.resource].References[tc.field]; ok {
				t.Errorf("%s: %s.%s unexpectedly references %s", scope, tc.resource, tc.field, ref.TerraformName)
			}
		}
	}
}

func refClient(t *testing.T, objs ...client.Object) client.Client {
	t.Helper()
	scheme := runtime.NewScheme()
	for _, add := range []func(*runtime.Scheme) error{
		domain.AddToScheme, org.AddToScheme, user.AddToScheme,
		clusterdomain.AddToScheme, clusterorg.AddToScheme, clusteruser.AddToScheme,
	} {
		if err := add(scheme); err != nil {
			t.Fatal(err)
		}
	}
	return fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
}

func TestDomainPolicyOrganizationSelector(t *testing.T) {
	policy := &domain.Policy{ObjectMeta: metav1.ObjectMeta{Name: "login-must-be-domain", Namespace: "tenant"}}
	policy.Spec.ForProvider.OrgIDSelector = &xpv1.NamespacedSelector{MatchLabels: tenantLabels}
	c := refClient(t, namespacedOrganization("tenant", "organization-id"), namespacedOrganization("other-tenant", "wrong-organization"))
	if err := policy.ResolveReferences(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if got := policy.Spec.ForProvider.OrgID; got == nil || *got != "organization-id" {
		t.Fatalf("resolved unexpected organization: %v", got)
	}
}

func TestClusterDomainPolicyOrganizationReference(t *testing.T) {
	organization := &clusterorg.Organization{ObjectMeta: metav1.ObjectMeta{
		Name:        "application",
		Annotations: map[string]string{"crossplane.io/external-name": "organization-id"},
	}}
	policy := &clusterdomain.Policy{ObjectMeta: metav1.ObjectMeta{Name: "login-must-be-domain"}}
	policy.Spec.ForProvider.OrgIDRef = &xpv1.Reference{Name: organization.Name}
	if err := policy.ResolveReferences(context.Background(), refClient(t, organization)); err != nil {
		t.Fatal(err)
	}
	if got := policy.Spec.ForProvider.OrgID; got == nil || *got != "organization-id" {
		t.Fatalf("resolved unexpected organization: %v", got)
	}
}

// Org members and org metadata keep resolving org_id from the Org kind, so
// existing references are unchanged. (Organization-managed orgs set orgId
// directly until an Organization-scoped administrator resource exists.)
func TestOrgMemberAndMetadataKeepOrgReference(t *testing.T) {
	for _, resource := range []string{"zitadel_org_member", "zitadel_org_metadata"} {
		assertReference(t, resource, "org_id", ujconfig.Reference{TerraformName: "zitadel_org"})
	}
}

func TestUserIDReferencesHumanUser(t *testing.T) {
	want := ujconfig.Reference{
		TerraformName:     "zitadel_human_user",
		RefFieldName:      "HumanUserIDRef",
		SelectorFieldName: "HumanUserIDSelector",
	}
	for _, resource := range []string{
		"zitadel_instance_member",
		"zitadel_org_member",
		"zitadel_project_grant_member",
		"zitadel_project_member",
		"zitadel_user_grant",
		"zitadel_user_metadata",
	} {
		t.Run(resource, func(t *testing.T) {
			assertReference(t, resource, "user_id", want)
		})
	}
}

var personaLabels = map[string]string{"example.org/persona": "owner"}

func TestOrgMemberHumanUserSelector(t *testing.T) {
	human := &user.HumanUser{ObjectMeta: metav1.ObjectMeta{
		Name: "owner", Namespace: "tenant", Labels: personaLabels,
		Annotations: map[string]string{"crossplane.io/external-name": "user-id"},
	}}
	other := &user.HumanUser{ObjectMeta: metav1.ObjectMeta{
		Name: "owner", Namespace: "other-tenant", Labels: personaLabels,
		Annotations: map[string]string{"crossplane.io/external-name": "wrong-user"},
	}}
	member := &org.Member{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "tenant"}}
	member.Spec.ForProvider.HumanUserIDSelector = &xpv1.NamespacedSelector{MatchLabels: personaLabels}
	c := refClient(t, human, other)
	if err := member.ResolveReferences(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if got := member.Spec.ForProvider.UserID; got == nil || *got != "user-id" {
		t.Fatalf("resolved unexpected user: %v", got)
	}
	if got := member.Spec.ForProvider.HumanUserIDRef; got == nil || got.Name != human.Name {
		t.Fatalf("reference was not persisted: %v", got)
	}
}

func TestClusterOrgMemberHumanUserReference(t *testing.T) {
	human := &clusteruser.HumanUser{ObjectMeta: metav1.ObjectMeta{
		Name:        "owner",
		Annotations: map[string]string{"crossplane.io/external-name": "user-id"},
	}}
	member := &clusterorg.Member{ObjectMeta: metav1.ObjectMeta{Name: "owner"}}
	member.Spec.ForProvider.HumanUserIDRef = &xpv1.Reference{Name: human.Name}
	if err := member.ResolveReferences(context.Background(), refClient(t, human)); err != nil {
		t.Fatal(err)
	}
	if got := member.Spec.ForProvider.UserID; got == nil || *got != "user-id" {
		t.Fatalf("resolved unexpected user: %v", got)
	}
}

// userId accepts any ZITADEL user, so a literal MachineUser ID must keep
// working alongside the HumanUser reference fields.
func TestUserGrantLiteralMachineUserID(t *testing.T) {
	machineUserID := "machine-user-id"
	grant := &user.Grant{ObjectMeta: metav1.ObjectMeta{Name: "automation", Namespace: "tenant"}}
	grant.Spec.ForProvider.UserID = &machineUserID
	if err := grant.ResolveReferences(context.Background(), refClient(t)); err != nil {
		t.Fatal(err)
	}
	if got := grant.Spec.ForProvider.UserID; got == nil || *got != machineUserID {
		t.Fatalf("literal user changed: %v", got)
	}
	if grant.Spec.ForProvider.HumanUserIDRef != nil {
		t.Fatalf("unexpected reference for literal user: %v", grant.Spec.ForProvider.HumanUserIDRef)
	}
}
