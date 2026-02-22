package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	"control/internal/config"
	"control/internal/store"
)

func main() {
	target := "019c8404-14a8-7b20-aa5f-184fe1a4e05d"
	if len(os.Args) > 1 {
		target = strings.TrimSpace(os.Args[1])
	}
	storagePath, err := config.StoragePath()
	if err != nil {
		panic(err)
	}
	repo, err := store.NewBboltRepository(storagePath)
	if err != nil {
		panic(err)
	}
	defer repo.Close()
	ctx := context.Background()

	records, err := repo.SessionIndex().ListRecords(ctx)
	if err != nil {
		panic(err)
	}
	sessions := make([]string, 0, len(records))
	providerByID := map[string]string{}
	statusByID := map[string]string{}
	for _, r := range records {
		if r == nil || r.Session == nil {
			continue
		}
		id := strings.TrimSpace(r.Session.ID)
		if id == "" {
			continue
		}
		sessions = append(sessions, id)
		providerByID[id] = strings.TrimSpace(r.Session.Provider)
		statusByID[id] = strings.TrimSpace(string(r.Session.Status))
	}

	meta, err := repo.SessionMeta().List(ctx)
	if err != nil {
		panic(err)
	}
	type mm struct{ thread, provider, run string }
	metaByID := map[string]mm{}
	for _, m := range meta {
		if m == nil {
			continue
		}
		id := strings.TrimSpace(m.SessionID)
		if id == "" {
			continue
		}
		metaByID[id] = mm{thread: strings.TrimSpace(m.ThreadID), provider: strings.TrimSpace(m.ProviderSessionID), run: strings.TrimSpace(m.WorkflowRunID)}
	}

	foundSession := false
	for _, id := range sessions {
		if id == target {
			foundSession = true
			fmt.Printf("session target found: id=%s provider=%s status=%s\n", id, providerByID[id], statusByID[id])
			break
		}
	}
	if !foundSession {
		fmt.Printf("session target not in session index: %s\n", target)
	}

	targetMeta, ok := metaByID[target]
	if ok {
		fmt.Printf("meta target found: id=%s thread=%s provider_session=%s workflow_run=%s\n", target, targetMeta.thread, targetMeta.provider, targetMeta.run)
	} else {
		fmt.Printf("meta target missing: %s\n", target)
	}

	thread := targetMeta.thread
	provider := targetMeta.provider
	type row struct{ id, provider, status, thread, psid, run string }
	rows := []row{}
	for _, id := range sessions {
		md := metaByID[id]
		match := id == target
		if thread != "" && (id == thread || md.thread == thread || md.provider == thread) {
			match = true
		}
		if provider != "" && (id == provider || md.provider == provider || md.thread == provider) {
			match = true
		}
		if match {
			rows = append(rows, row{id: id, provider: providerByID[id], status: statusByID[id], thread: md.thread, psid: md.provider, run: md.run})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].id < rows[j].id })
	fmt.Printf("matching sessions count=%d\n", len(rows))
	for _, r := range rows {
		fmt.Printf("  session=%s provider=%s status=%s thread=%s provider_session=%s run=%s\n", r.id, r.provider, r.status, r.thread, r.psid, r.run)
	}
}
