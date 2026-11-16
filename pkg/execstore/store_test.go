/**********************************************************************
 * Copyright (C) 2026 Red Hat, Inc.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * SPDX-License-Identifier: Apache-2.0
 **********************************************************************/

package execstore

import "testing"

func TestExecLifecycleAndSingleStart(t *testing.T) {
	t.Parallel()
	store := NewStore()
	created, err := store.Create("container-id", []string{"sh", "-c", "exit 42"})
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.State != Created {
		t.Fatalf("unexpected initial record: %#v", created)
	}
	started, err := store.Start(created.ID)
	if err != nil || started.State != Running {
		t.Fatalf("Start() = (%#v, %v)", started, err)
	}
	if _, err := store.Start(created.ID); err == nil {
		t.Fatal("a running exec was started a second time")
	}
	completed, err := store.Complete(created.ID, 42)
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != Exited || completed.ExitCode == nil || *completed.ExitCode != 42 {
		t.Fatalf("unexpected completed record: %#v", completed)
	}
}

func TestStoreRejectsUnknownIDs(t *testing.T) {
	t.Parallel()
	store := NewStore()
	if _, err := store.Start("missing"); err == nil {
		t.Fatal("Start() accepted an unknown exec ID")
	}
	if _, err := store.Complete("missing", 0); err == nil {
		t.Fatal("Complete() accepted an unknown exec ID")
	}
	if _, ok := store.Get("missing"); ok {
		t.Fatal("Get() found an unknown exec ID")
	}
}

func TestStoreFailMarksRecordFailed(t *testing.T) {
	t.Parallel()
	store := NewStore()
	created, err := store.Create("container-id", []string{"ls"})
	if err != nil {
		t.Fatal(err)
	}
	store.Fail(created.ID)
	record, ok := store.Get(created.ID)
	if !ok || record.State != Failed {
		t.Fatalf("Get() after Fail() = (%#v, %t), want a failed record", record, ok)
	}
}

func TestStoreFailIgnoresUnknownIDs(t *testing.T) {
	t.Parallel()
	store := NewStore()
	store.Fail("missing")
}

func TestStoreEnforcesRecordCapacity(t *testing.T) {
	t.Parallel()
	store := NewStore()
	for count := 0; count < maxRecords; count++ {
		if _, err := store.Create("container-id", []string{"ls"}); err != nil {
			t.Fatalf("Create() number %d failed: %v", count+1, err)
		}
	}
	if _, err := store.Create("container-id", []string{"ls"}); err == nil {
		t.Fatal("Create() exceeded the record capacity")
	}
}
