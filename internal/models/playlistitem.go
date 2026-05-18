package models

type PlayListItem struct {
	ID          string
	Title       string
	PublishedAt string
	ChannelID   string
	Description string
	VideoID     string
}

func NewPlayListItem(id, title, publishedAt, channelID, description, videoID string) PlayListItem {
	return PlayListItem{
		ID:          id,
		Title:       title,
		PublishedAt: publishedAt,
		ChannelID:   channelID,
		Description: description,
		VideoID:     videoID,
	}
}
