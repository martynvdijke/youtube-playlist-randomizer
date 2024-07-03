from youtube_playlist_randomizer.playlistitem import PlayListItem
from youtube_playlist_randomizer.playlist import Playlist


def test_playlist_initialization():
    # Test data
    id = "playlist123"
    title = "My Playlist"

    # Create an instance of Playlist
    playlist = Playlist(id, title)

    # Assert statements to check if the object is initialized correctly
    assert playlist.id == id
    assert playlist.title == title
    assert playlist.items == []


def test_playlist_add_item():
    # Playlist test data
    playlist_id = "playlist123"
    playlist_title = "My Playlist"
    playlist = Playlist(playlist_id, playlist_title)

    # PlayListItem test data
    item_id = "123"
    item_title = "Test Title"
    publishedAt = "2021-01-01T00:00:00Z"
    channelId = "channel123"
    description = "This is a test description"
    videoId = "video123"

    # Create an instance of PlayListItem
    item = PlayListItem(
        item_id, item_title, publishedAt, channelId, description, videoId
    )

    # Add the item to the playlist
    playlist.add_item(item)

    # Assert statements to check if the item was added correctly
    assert len(playlist.items) == 1
    assert playlist.items[0] == item


def test_playlist_add_multiple_items():
    # Playlist test data
    playlist_id = "playlist123"
    playlist_title = "My Playlist"
    playlist = Playlist(playlist_id, playlist_title)

    # PlayListItem test data
    item1 = PlayListItem(
        "1", "Title 1", "2021-01-01T00:00:00Z", "channel1", "Description 1", "video1"
    )
    item2 = PlayListItem(
        "2", "Title 2", "2021-02-01T00:00:00Z", "channel2", "Description 2", "video2"
    )
    item3 = PlayListItem(
        "3", "Title 3", "2021-03-01T00:00:00Z", "channel3", "Description 3", "video3"
    )

    # Add items to the playlist
    playlist.add_item(item1)
    playlist.add_item(item2)
    playlist.add_item(item3)

    # Assert statements to check if the items were added correctly
    assert len(playlist.items) == 3
    assert playlist.items[0] == item1
    assert playlist.items[1] == item2
    assert playlist.items[2] == item3
