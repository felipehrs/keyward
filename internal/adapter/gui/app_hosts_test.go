package main

import "testing"

func TestApp_ListHosts_Empty(t *testing.T) {
	a := newTestApp(t)
	hosts, err := a.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("esperava lista vazia, obteve %+v", hosts)
	}
}

func TestApp_AddHost_ThenListHosts(t *testing.T) {
	a := newTestApp(t)
	if err := a.AddHost(HostSpecInput{Patterns: []string{"bastion"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}

	hosts, err := a.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts: %v", err)
	}
	if len(hosts) != 1 {
		t.Fatalf("esperava 1 host, obteve %d", len(hosts))
	}
	if hosts[0].Patterns[0] != "bastion" || hosts[0].HostName != "1.2.3.4" {
		t.Fatalf("host inesperado: %+v", hosts[0])
	}
}

func TestApp_AddHost_DuplicatePatterns_ReturnsError(t *testing.T) {
	a := newTestApp(t)
	spec := HostSpecInput{Patterns: []string{"bastion"}, HostName: "1.2.3.4"}
	if err := a.AddHost(spec); err != nil {
		t.Fatalf("AddHost inicial: %v", err)
	}
	if err := a.AddHost(spec); err == nil {
		t.Fatal("esperava erro ao adicionar host com Patterns já existente")
	}
}

func TestApp_AddHost_EmptyPatterns_ReturnsError(t *testing.T) {
	a := newTestApp(t)
	if err := a.AddHost(HostSpecInput{}); err == nil {
		t.Fatal("esperava erro para HostSpec sem Patterns")
	}
}

func TestApp_ReplaceHost_ReplacesExistingBlock(t *testing.T) {
	a := newTestApp(t)
	if err := a.AddHost(HostSpecInput{Patterns: []string{"bastion"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	hosts, err := a.ListHosts()
	if err != nil || len(hosts) != 1 {
		t.Fatalf("ListHosts: %v %+v", err, hosts)
	}

	err = a.ReplaceHost(ReplaceHostInput{
		SourceFile:  hosts[0].SourceFile,
		OldPatterns: hosts[0].Patterns,
		NewSpec:     HostSpecInput{Patterns: []string{"bastion"}, HostName: "9.9.9.9", User: "ops"},
	})
	if err != nil {
		t.Fatalf("ReplaceHost: %v", err)
	}

	hosts, err = a.ListHosts()
	if err != nil || len(hosts) != 1 {
		t.Fatalf("ListHosts após replace: %v %+v", err, hosts)
	}
	if hosts[0].HostName != "9.9.9.9" || hosts[0].User != "ops" {
		t.Fatalf("host não refletiu a substituição: %+v", hosts[0])
	}
}

func TestApp_ReplaceHost_MissingHost_ReturnsError(t *testing.T) {
	a := newTestApp(t)
	err := a.ReplaceHost(ReplaceHostInput{
		OldPatterns: []string{"nao-existe"},
		NewSpec:     HostSpecInput{Patterns: []string{"nao-existe"}, HostName: "1.2.3.4"},
	})
	if err == nil {
		t.Fatal("esperava erro ao substituir host inexistente")
	}
}

func TestApp_RemoveHost_RemovesExistingBlock(t *testing.T) {
	a := newTestApp(t)
	if err := a.AddHost(HostSpecInput{Patterns: []string{"bastion"}, HostName: "1.2.3.4"}); err != nil {
		t.Fatalf("AddHost: %v", err)
	}
	hosts, err := a.ListHosts()
	if err != nil || len(hosts) != 1 {
		t.Fatalf("ListHosts: %v %+v", err, hosts)
	}

	if err := a.RemoveHost(RemoveHostInput{SourceFile: hosts[0].SourceFile, Patterns: hosts[0].Patterns}); err != nil {
		t.Fatalf("RemoveHost: %v", err)
	}

	hosts, err = a.ListHosts()
	if err != nil {
		t.Fatalf("ListHosts após remove: %v", err)
	}
	if len(hosts) != 0 {
		t.Fatalf("esperava lista vazia após remove, obteve %+v", hosts)
	}
}

func TestApp_RemoveHost_MissingHost_ReturnsError(t *testing.T) {
	a := newTestApp(t)
	if err := a.RemoveHost(RemoveHostInput{Patterns: []string{"nao-existe"}}); err == nil {
		t.Fatal("esperava erro ao remover host inexistente")
	}
}
