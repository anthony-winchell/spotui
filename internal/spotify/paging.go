package spotify

// Page is one page of a longer list
type Page[T any] struct {
	Items   []T
	Offset  int  // position of Items[0] in the full list
	Total   int  // number of items across all pages
	HasNext bool // true if another page follows this one
}

// pagingResponse is Spotify's JSON paging object
type pagingResponse[T any] struct {
	Items  []T     `json:"items"`
	Offset int     `json:"offset"`
	Total  int     `json:"total"`
	Next   *string `json:"next"` // null on the last page
}

// mapPage converts a decoded JSON page into a Page, converting each item
func mapPage[T, U any](p pagingResponse[T], convert func(T) U) Page[U] {
	items := make([]U, len(p.Items))
	for i, it := range p.Items {
		items[i] = convert(it)
	}

	return Page[U]{
		Items:   items,
		Offset:  p.Offset,
		Total:   p.Total,
		HasNext: p.Next != nil,
	}
}
