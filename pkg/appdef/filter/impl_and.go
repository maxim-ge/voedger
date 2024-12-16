/*
 * Copyright (c) 2024-present Sigma-Soft, Ltd.
 * @author: Nikolay Nikitin
 */

package filter

import (
	"fmt"
	"iter"
	"slices"

	"github.com/voedger/voedger/pkg/appdef"
)

// orFilter realizes filter conjunction.
//
// # Supports:
//   - appdef.IFilter.
//   - fmt.Stringer
type andFilter struct {
	filter
	children []appdef.IFilter
}

func makeAndFilter(f1, f2 appdef.IFilter, ff ...appdef.IFilter) appdef.IFilter {
	f := &andFilter{children: []appdef.IFilter{f1, f2}}
	f.children = append(f.children, ff...)
	return f
}

func (f andFilter) And() iter.Seq[appdef.IFilter] { return slices.Values(f.children) }

func (andFilter) Kind() appdef.FilterKind { return appdef.FilterKind_And }

func (f andFilter) Match(t appdef.IType) bool {
	for c := range f.And() {
		if !c.Match(t) {
			return false
		}
	}
	return true
}

func (f andFilter) String() string {
	// QNAMES(…) AND TAGS(…)
	// (QNAMES(…) OR TYPES(…)) AND NOT TAGS(…)
	s := ""
	for i, c := range f.children {
		cStr := fmt.Sprint(c)
		if (c.Kind() == appdef.FilterKind_Or) || (c.Kind() == appdef.FilterKind_And) {
			cStr = fmt.Sprintf("(%s)", cStr)
		}
		if i > 0 {
			s += " AND "
		}
		s += cStr
	}
	return s
}