import pytest
from unittest.mock import MagicMock
from youtube_playlist_randomizer.playlist import Playlist
from youtube_playlist_randomizer.randomizer import Randomizer


@pytest.fixture
def youtube_mock():
    return MagicMock()


@pytest.fixture
def args_mock():
    return MagicMock(chunks=10)


@pytest.fixture
def randomizer(youtube_mock, args_mock):
    return Randomizer(youtube_mock, args_mock)


def test_randomizer_initialization(randomizer, youtube_mock, args_mock):
    assert randomizer.youtube == youtube_mock
    assert randomizer.chunks == args_mock.chunks


def test_convert_to_playlists(randomizer):
    response_mock = {
        "items": [
            {"id": "playlist1", "snippet": {"localized": {"title": "Playlist 1"}}},
            {"id": "playlist2", "snippet": {"localized": {"title": "Playlist 2"}}},
        ]
    }
    randomizer._get_playlists = MagicMock(return_value=response_mock)
    playlists = randomizer.convert_to_playlists()
    assert len(playlists) == 2
    assert playlists[0].id == "playlist1"
    assert playlists[0].title == "Playlist 1"
    assert playlists[1].id == "playlist2"
    assert playlists[1].title == "Playlist 2"


def test_create_youtube_playlist(randomizer, youtube_mock):
    response_mock = {"id": "new_playlist_id"}
    youtube_mock.playlists().insert().execute.return_value = response_mock
    playlist_id = randomizer.create_youtube_playlist("New Playlist")
    assert playlist_id == "new_playlist_id"


def test_convert_to_playlist_items(randomizer):
    playlist = Playlist("playlist1", "Playlist 1")
    response_mock = {
        "items": [
            {
                "id": "item1",
                "snippet": {
                    "resourceId": {"videoId": "video1"},
                    "publishedAt": "2021-01-01T00:00:00Z",
                    "channelId": "channel1",
                    "description": "Description 1",
                    "title": "Title 1",
                },
            }
        ],
        "nextPageToken": "next_page_token",
    }
    next_page = randomizer.convert_to_playlist_items(playlist, response_mock)
    assert len(playlist.items) == 1
    assert playlist.items[0].id == "item1"
    assert next_page == "next_page_token"


def test_get_id(randomizer):
    playlist1 = Playlist("playlist1", "Playlist 1")
    playlist2 = Playlist("playlist2", "Playlist 2")
    randomizer.playlists = [playlist1, playlist2]
    playlist = randomizer.get_id("Playlist 1")
    assert playlist == playlist1
