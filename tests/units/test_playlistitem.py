from youtube_playlist_randomizer.playlistitem import PlayListItem


def test_playlist_item_initialization():
    # Test data
    id = "123"
    title = "Test Title"
    publishedAt = "2021-01-01T00:00:00Z"
    channelId = "channel123"
    description = "This is a test description"
    videoId = "video123"

    # Create an instance of PlayListItem
    item = PlayListItem(id, title, publishedAt, channelId, description, videoId)

    # Assert statements to check if the object is initialized correctly
    assert item.id == id
    assert item.title == title
    assert item.publishedAt == publishedAt
    assert item.channelId == channelId
    assert item.description == description
    assert item.videoId == videoId


def test_playlist_item_empty_initialization():
    # Test empty strings
    item = PlayListItem("", "", "", "", "", "")

    # Assert statements to check if the object is initialized with empty strings correctly
    assert item.id == ""
    assert item.title == ""
    assert item.publishedAt == ""
    assert item.channelId == ""
    assert item.description == ""
    assert item.videoId == ""


def test_playlist_item_partial_initialization():
    # Test partial data
    id = "123"
    title = "Test Title"
    item = PlayListItem(id, title, "", "", "", "")

    # Assert statements to check if the object is initialized correctly
    assert item.id == id
    assert item.title == title
    assert item.publishedAt == ""
    assert item.channelId == ""
    assert item.description == ""
    assert item.videoId == ""
