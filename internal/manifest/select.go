package manifest

import (
	"fmt"
	"sort"
	"strings"
)

func (d *Document) Select(selectors []string) ([]Selection, error) {
	if d.resolved == nil {
		if _, err := d.Validate(false); err != nil {
			return nil, err
		}
	}
	selected := make(map[string]Selection)
	if len(selectors) == 0 {
		for id, selection := range d.resolved {
			selected[id] = selection
		}
	} else {
		for _, selector := range selectors {
			parts := strings.Split(selector, "/")
			if len(parts) > 2 || parts[0] == "" || (len(parts) == 2 && parts[1] == "") {
				return nil, fmt.Errorf("invalid selector %q", selector)
			}
			resource, ok := d.Manifest.Resources[parts[0]]
			if !ok {
				return nil, fmt.Errorf("unknown resource %q", parts[0])
			}
			if len(parts) == 2 {
				id := parts[0] + "/" + parts[1]
				selection, ok := d.resolved[id]
				if !ok {
					return nil, fmt.Errorf("unknown download %q in resource %q", parts[1], parts[0])
				}
				selected[id] = selection
				continue
			}
			for downloadName := range resource.Downloads {
				id := parts[0] + "/" + downloadName
				selected[id] = d.resolved[id]
			}
		}
	}
	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	result := make([]Selection, 0, len(ids))
	for _, id := range ids {
		result = append(result, selected[id])
	}
	return result, nil
}
