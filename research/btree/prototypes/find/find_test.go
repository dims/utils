// Copyright 2025 The Kubernetes Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package btree

import (
	"cmp"
	"math/rand"
	"testing"
)

// TestFindMatchesGet checks Find, FindBisect and GetFunc against Get for
// every key in and around a tree, at degrees that give one to four levels.
func TestFindMatchesGet(t *testing.T) {
	for _, degree := range []int{2, 3, 16} {
		for _, n := range []int{0, 1, 5, 100, 5000} {
			tr := NewOrdered[int](degree)
			for _, v := range rand.Perm(n) {
				tr.ReplaceOrInsert(v * 2)
			}
			for k := -2; k <= 2*n+2; k++ {
				want, wantOK := tr.Get(k)
				if got, ok := Find(tr, k, cmp.Compare[int]); ok != wantOK || got != want {
					t.Fatalf("degree %d n %d Find(%d) = %d %v, Get %d %v", degree, n, k, got, ok, want, wantOK)
				}
				if got, ok := FindBisect(tr, k, cmp.Compare[int]); ok != wantOK || got != want {
					t.Fatalf("degree %d n %d FindBisect(%d) = %d %v, Get %d %v", degree, n, k, got, ok, want, wantOK)
				}
				if got, ok := tr.GetFunc(func(item int) int { return cmp.Compare(item, k) }); ok != wantOK || got != want {
					t.Fatalf("degree %d n %d GetFunc(%d) = %d %v, Get %d %v", degree, n, k, got, ok, want, wantOK)
				}
			}
		}
	}
}
