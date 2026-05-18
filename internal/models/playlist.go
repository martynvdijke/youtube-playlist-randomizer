package models

type Playlist struct {
	ID        string
	Title     string
	Items     []PlayListItem
	ItemCount int
}

func NewPlaylist(id, title string) *Playlist {
	return &Playlist{
		ID:    id,
		Title: title,
		Items: make([]PlayListItem, 0),
	}
}

func (p *Playlist) AddItem(item PlayListItem) {
	p.Items = append(p.Items, item)
}
