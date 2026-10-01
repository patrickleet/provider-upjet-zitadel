// SPDX-FileCopyrightText: 2026 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"context"
	"strings"
	"sync"
	"testing"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	ujconfig "github.com/crossplane/upjet/v2/pkg/config"
	"github.com/crossplane/upjet/v2/pkg/resource/kindref"
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

// userId accepts any ZITADEL user: userIdRef and userIdSelector resolve a
// HumanUser by default, or a MachineUser with kind: MachineUser.
func TestUserIDReferencesHumanUserOrMachineUser(t *testing.T) {
	for _, resource := range []string{
		"zitadel_instance_member",
		"zitadel_org_member",
		"zitadel_project_grant_member",
		"zitadel_project_member",
		"zitadel_user_grant",
		"zitadel_user_metadata",
	} {
		t.Run(resource, func(t *testing.T) {
			assertReference(t, resource, "user_id", ujconfig.Reference{TerraformName: "zitadel_human_user"})
			for scope, p := range bothProviders() {
				got := p.Resources[resource].References["user_id"].Targets()
				if len(got) != 2 || got[1].TerraformName != "zitadel_machine_user" {
					t.Errorf("%s: %s.user_id additional targets = %+v, want only zitadel_machine_user", scope, resource, got)
				}
			}
		})
	}
}

var personaLabels = map[string]string{"example.org/persona": "owner"}

const (
	userAPIVersion        = "user.zitadel.m.crossplane.io/v1alpha1"
	clusterUserAPIVersion = "user.zitadel.crossplane.io/v1alpha1"
	unsupportedTargetErr  = `: must be one of: user.zitadel.m.crossplane.io/v1alpha1 HumanUser (default), user.zitadel.m.crossplane.io/v1alpha1 MachineUser`
)

func humanUser(namespace, name, externalName string, labels map[string]string) *user.HumanUser {
	return &user.HumanUser{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: namespace, Labels: labels,
		Annotations: map[string]string{"crossplane.io/external-name": externalName},
	}}
}

func machineUser(namespace, name, externalName string, labels map[string]string) *user.MachineUser {
	return &user.MachineUser{ObjectMeta: metav1.ObjectMeta{
		Name: name, Namespace: namespace, Labels: labels,
		Annotations: map[string]string{"crossplane.io/external-name": externalName},
	}}
}

// Both kinds share a name and labels in each test, so a test passes only if
// the requested kind is the one resolved.
func userObjects() []client.Object {
	return []client.Object{
		humanUser("tenant", "alice", "human-user-id", personaLabels),
		machineUser("tenant", "alice", "machine-user-id", personaLabels),
		humanUser("other-tenant", "alice", "wrong-human-user", personaLabels),
		machineUser("other-tenant", "alice", "wrong-machine-user", personaLabels),
	}
}

func TestUserGrantUserReference(t *testing.T) {
	cases := map[string]struct {
		ref         *kindref.NamespacedReference
		wantUserID  string
		wantRefKind string
		wantErr     string
	}{
		"KindOmittedResolvesHumanUser": {
			ref:        &kindref.NamespacedReference{Name: "alice"},
			wantUserID: "human-user-id",
		},
		"HumanUser": {
			ref:         &kindref.NamespacedReference{Kind: "HumanUser", Name: "alice"},
			wantUserID:  "human-user-id",
			wantRefKind: "HumanUser",
		},
		"MachineUser": {
			ref:         &kindref.NamespacedReference{Kind: "MachineUser", Name: "alice"},
			wantUserID:  "machine-user-id",
			wantRefKind: "MachineUser",
		},
		"APIVersionAndKind": {
			ref:         &kindref.NamespacedReference{APIVersion: userAPIVersion, Kind: "MachineUser", Name: "alice"},
			wantUserID:  "machine-user-id",
			wantRefKind: "MachineUser",
		},
		"UnknownKind": {
			ref:     &kindref.NamespacedReference{Kind: "ServiceAccount", Name: "alice"},
			wantErr: `mg.Spec.ForProvider.UserID: unsupported reference target apiVersion "", kind "ServiceAccount"` + unsupportedTargetErr,
		},
		// The cluster-scoped MachineUser has the same kind in another group.
		"APIVersionMismatch": {
			ref:     &kindref.NamespacedReference{APIVersion: clusterUserAPIVersion, Kind: "MachineUser", Name: "alice"},
			wantErr: `mg.Spec.ForProvider.UserID: unsupported reference target apiVersion "user.zitadel.crossplane.io/v1alpha1", kind "MachineUser"` + unsupportedTargetErr,
		},
		"APIVersionWithoutKind": {
			ref:     &kindref.NamespacedReference{APIVersion: userAPIVersion, Name: "alice"},
			wantErr: `mg.Spec.ForProvider.UserID: unsupported reference target apiVersion "user.zitadel.m.crossplane.io/v1alpha1", kind ""` + unsupportedTargetErr,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			grant := &user.Grant{ObjectMeta: metav1.ObjectMeta{Name: "grant", Namespace: "tenant"}}
			grant.Spec.ForProvider.UserIDRef = tc.ref
			err := grant.ResolveReferences(context.Background(), refClient(t, userObjects()...))
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Fatalf("ResolveReferences(): want error %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := grant.Spec.ForProvider.UserID; got == nil || *got != tc.wantUserID {
				t.Fatalf("resolved unexpected user: %v", got)
			}
			if got := grant.Spec.ForProvider.UserIDRef; got == nil || got.Name != "alice" || got.Kind != tc.wantRefKind {
				t.Fatalf("reference not persisted as given: %+v", got)
			}
		})
	}
}

func TestOrgMemberUserSelector(t *testing.T) {
	cases := map[string]struct {
		sel         *kindref.NamespacedSelector
		wantUserID  string
		wantRefKind string
	}{
		"KindOmittedSelectsHumanUser": {
			sel:        &kindref.NamespacedSelector{MatchLabels: personaLabels},
			wantUserID: "human-user-id",
		},
		"MachineUser": {
			sel:         &kindref.NamespacedSelector{Kind: "MachineUser", MatchLabels: personaLabels},
			wantUserID:  "machine-user-id",
			wantRefKind: "MachineUser",
		},
		"APIVersionAndKind": {
			sel:         &kindref.NamespacedSelector{APIVersion: userAPIVersion, Kind: "MachineUser", MatchLabels: personaLabels},
			wantUserID:  "machine-user-id",
			wantRefKind: "MachineUser",
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			member := &org.Member{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "tenant"}}
			member.Spec.ForProvider.UserIDSelector = tc.sel
			if err := member.ResolveReferences(context.Background(), refClient(t, userObjects()...)); err != nil {
				t.Fatal(err)
			}
			if got := member.Spec.ForProvider.UserID; got == nil || *got != tc.wantUserID {
				t.Fatalf("resolved unexpected user: %v", got)
			}
			// The selected reference is persisted with the selector's kind,
			// so later reconciles resolve the same kind.
			ref := member.Spec.ForProvider.UserIDRef
			if ref == nil || ref.Name != "alice" || ref.Kind != tc.wantRefKind || ref.APIVersion != tc.sel.APIVersion {
				t.Fatalf("selected reference was not persisted: %+v", ref)
			}
		})
	}
}

// A selector with the Always resolve policy selects again on every
// reconcile, so its kind wins over the kind of the reference it set before.
func TestOrgMemberAlwaysSelectorKindWins(t *testing.T) {
	always := xpv1.ResolvePolicyAlways
	member := &org.Member{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "tenant"}}
	member.Spec.ForProvider.UserID = ptrTo("human-user-id")
	member.Spec.ForProvider.UserIDRef = &kindref.NamespacedReference{Kind: "HumanUser", Name: "alice"}
	member.Spec.ForProvider.UserIDSelector = &kindref.NamespacedSelector{
		Kind: "MachineUser", MatchLabels: personaLabels, Policy: &xpv1.Policy{Resolve: &always},
	}
	if err := member.ResolveReferences(context.Background(), refClient(t, userObjects()...)); err != nil {
		t.Fatal(err)
	}
	if got := member.Spec.ForProvider.UserID; got == nil || *got != "machine-user-id" {
		t.Fatalf("resolved unexpected user: %v", got)
	}
	if got := member.Spec.ForProvider.UserIDRef; got == nil || got.Kind != "MachineUser" {
		t.Fatalf("reference kind not updated: %+v", got)
	}
}

// Only the configured targets are looked up: a same-named kind in another
// group (here the cluster-scoped MachineUser) is never resolved.
func TestUserReferenceIgnoresUnrelatedSameNamedKind(t *testing.T) {
	unrelated := &clusteruser.MachineUser{ObjectMeta: metav1.ObjectMeta{
		Name: "ci-bot", Labels: personaLabels,
		Annotations: map[string]string{"crossplane.io/external-name": "unrelated-user"},
	}}
	t.Run("Reference", func(t *testing.T) {
		member := &org.Member{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "tenant"}}
		member.Spec.ForProvider.UserIDRef = &kindref.NamespacedReference{Kind: "MachineUser", Name: "ci-bot"}
		err := member.ResolveReferences(context.Background(), refClient(t, unrelated))
		if err == nil || !strings.Contains(err.Error(), "cannot get referenced resource") {
			t.Fatalf("ResolveReferences(): want a not found error, got %v (user %v)", err, member.Spec.ForProvider.UserID)
		}
	})
	t.Run("Selector", func(t *testing.T) {
		member := &org.Member{ObjectMeta: metav1.ObjectMeta{Name: "owner", Namespace: "tenant"}}
		member.Spec.ForProvider.UserIDSelector = &kindref.NamespacedSelector{Kind: "MachineUser", MatchLabels: personaLabels}
		err := member.ResolveReferences(context.Background(), refClient(t, unrelated))
		if err == nil || !strings.Contains(err.Error(), "no resources matched selector") {
			t.Fatalf("ResolveReferences(): want a no match error, got %v (user %v)", err, member.Spec.ForProvider.UserID)
		}
	})
}

func TestClusterOrgMemberUserReference(t *testing.T) {
	human := &clusteruser.HumanUser{ObjectMeta: metav1.ObjectMeta{
		Name:        "owner",
		Annotations: map[string]string{"crossplane.io/external-name": "human-user-id"},
	}}
	machine := &clusteruser.MachineUser{ObjectMeta: metav1.ObjectMeta{
		Name:        "owner",
		Annotations: map[string]string{"crossplane.io/external-name": "machine-user-id"},
	}}
	for kind, want := range map[string]string{"": "human-user-id", "HumanUser": "human-user-id", "MachineUser": "machine-user-id"} {
		member := &clusterorg.Member{ObjectMeta: metav1.ObjectMeta{Name: "owner"}}
		member.Spec.ForProvider.UserIDRef = &kindref.Reference{Kind: kind, Name: "owner"}
		if err := member.ResolveReferences(context.Background(), refClient(t, human, machine)); err != nil {
			t.Fatal(err)
		}
		if got := member.Spec.ForProvider.UserID; got == nil || *got != want {
			t.Fatalf("kind %q: resolved unexpected user: %v", kind, got)
		}
	}
	member := &clusterorg.Member{ObjectMeta: metav1.ObjectMeta{Name: "owner"}}
	member.Spec.ForProvider.UserIDRef = &kindref.Reference{APIVersion: userAPIVersion, Kind: "MachineUser", Name: "owner"}
	err := member.ResolveReferences(context.Background(), refClient(t, human, machine))
	want := `mg.Spec.ForProvider.UserID: unsupported reference target apiVersion "user.zitadel.m.crossplane.io/v1alpha1", kind "MachineUser": must be one of: user.zitadel.crossplane.io/v1alpha1 HumanUser (default), user.zitadel.crossplane.io/v1alpha1 MachineUser`
	if err == nil || err.Error() != want {
		t.Fatalf("ResolveReferences(): want error %q, got %v", want, err)
	}
}

// A literal user ID keeps working without a reference.
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
	if grant.Spec.ForProvider.UserIDRef != nil {
		t.Fatalf("unexpected reference for literal user: %v", grant.Spec.ForProvider.UserIDRef)
	}
}

func ptrTo[T any](v T) *T { return &v }
