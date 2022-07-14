/*
 * Copyright (C) 2020-2022, IrineSistiana
 *
 * This file is part of mosdns.
 *
 * mosdns is free software: you can redistribute it and/or modify
 * it under the terms of the GNU General Public License as published by
 * the Free Software Foundation, either version 3 of the License, or
 * (at your option) any later version.
 *
 * mosdns is distributed in the hope that it will be useful,
 * but WITHOUT ANY WARRANTY; without even the implied warranty of
 * MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
 * GNU General Public License for more details.
 *
 * You should have received a copy of the GNU General Public License
 * along with this program.  If not, see <https://www.gnu.org/licenses/>.
 */

package list

import (
	"github.com/stretchr/testify/assert"
	"reflect"
	"testing"
)

func checkLinkPointers[V any](t *testing.T, l *List[V]) {
	t.Helper()

	e := l.front
	for e != nil {
		if (e.next != nil && e.next.prev != e) || (e.prev != nil && e.prev.next != e) {
			t.Fatal("broken list")
		}
		e = e.next
	}
}

func makeElems(n []int) []*Elem[int] {
	s := make([]*Elem[int], 0, len(n))
	for _, i := range n {
		s = append(s, NewElem(i))
	}
	return s
}

func allValue[V any](l *List[V]) []V {
	s := make([]V, 0)
	node := l.front
	for node != nil {
		s = append(s, node.Value)
		node = node.next
	}
	return s
}

func TestList_Push(t *testing.T) {
	l := new(List[int])
	l.PushBack(NewElem(1))
	l.PushBack(NewElem(2))
	assert.Equal(t, []int{1, 2}, allValue(l))
	checkLinkPointers(t, l)

	l = new(List[int])
	l.PushFront(NewElem(1))
	l.PushFront(NewElem(2))
	assert.Equal(t, []int{2, 1}, allValue(l))
	checkLinkPointers(t, l)
}

func TestList_PopElem(t *testing.T) {
	tests := []struct {
		name     string
		in       []int
		pop      int
		want     int
		wantList []int
	}{
		{"pop front", []int{0, 1, 2}, 0, 0, []int{1, 2}},
		{"pop mid", []int{0, 1, 2}, 1, 1, []int{0, 2}},
		{"pop back", []int{0, 1, 2}, 2, 2, []int{0, 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := new(List[int])
			le := makeElems(tt.in)
			for _, e := range le {
				l.PushBack(e)
			}
			checkLinkPointers(t, l)

			got := l.PopElem(le[tt.pop])
			checkLinkPointers(t, l)

			if got.Value != tt.want {
				t.Errorf("PopElem() = %v, want %v", got.Value, tt.want)
			}
			if !reflect.DeepEqual(allValue(l), tt.wantList) {
				t.Errorf("allValue() = %v, want %v", allValue(l), tt.wantList)
			}
		})
	}
}
