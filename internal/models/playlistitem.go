package models

type PlayListItem struct {
	ID           string
	Title        string
	PublishedAt  string
	ChannelID    string
	ChannelTitle string
	Description  string
	VideoID      string
	ThumbnailURL string
}

func NewPlayListItem(id, title, publishedAt, channelID, channelTitle, description, videoID, thumbnailURL string) PlayListItem {
	return PlayListItem{
		ID:           id,
		Title:        title,
		PublishedAt:  publishedAt,
		ChannelID:    channelID,
		ChannelTitle: channelTitle,
		Description:  description,
		VideoID:      videoID,
		ThumbnailURL: thumbnailURL,
	}
}
