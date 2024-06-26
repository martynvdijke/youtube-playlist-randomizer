"""
File where all the magic happens
"""

import logging
import random
import time
from typing import Optional
from googleapiclient.errors import HttpError
from prompt_toolkit import prompt
from prompt_toolkit.completion import WordCompleter
from youtube_playlist_randomizer.playlist import Playlist
from youtube_playlist_randomizer.playlistitem import PlayListItem

_logger = logging.getLogger(__name__)

# pylint: disable=C0200


class Randomizer:
    """
    Playlist data class
    """

    def __init__(self, youtube, args):
        """
        Initializes playlist class.
        """
        self.youtube = youtube
        self.chunks = args.chunks

    def run(self):
        """
        Runs the program
        """
        self.playlists = self.convert_to_playlists()
        playlist = self.choose_playlist()
        self.randomize_playlist(playlist)

    def convert_to_playlists(self) -> list:
        """
        Converts the API results to Playlists

        Returns:
            list: list of Playlists
        """
        response = self._get_playlists()
        playlists = []

        _logger.debug(response)
        for item in response["items"]:
            id = item["id"]
            title = item["snippet"]["localized"]["title"]
            playlist = Playlist(id, title)
            playlists.append(playlist)

        return playlists

    def _get_playlists(self) -> dict:
        """
        Gets the playlists

        Returns:
            dict: API response
        """
        request = self.youtube.playlists().list(part="snippet", mine=True)
        response = request.execute()
        return response

    def create_youtube_playlist(self, title):
        """
        Makes a new playlis to hold the randomized videos
        :param title:
        :return:
        """
        request = self.youtube.playlists().insert(
            part="snippet,status", body={"snippet": {"title": title}}
        )
        try:
            response = request.execute()
            id = response["id"]
            return id
        except HttpError as error:
            _logger.warning(
                "Error in making new playlist status code: %s details: %s",
                error.status_code,
                error.error_details,
            )

    def randomize_playlist(self, playlist: Playlist):
        """
        Randomizes the given playlists

        Args:
            playlist (Playlist): Playlist object
        """
        current_items = 0
        current_items, total_results, next_page = self.initial_list_playlist_items(
            playlist, current_items
        )

        while current_items <= total_results:
            current_items, next_page = self.list_playlist_items(
                playlist, current_items, next_page
            )

        self.populate_new_playlist(playlist)

    def list_playlist_items(
        self, playlist: Playlist, current_items: int, next_page: str
    ):
        """
        Lists the items in the playlist

        Args:
            playlist (Playlist): Playlist object
            current_items (int): Number of items currently processed
            next_page (str): Next page token

        Returns:
            _type_: _description_
        """
        id = playlist.id
        request = self.youtube.playlistItems().list(
            part="snippet", pageToken=next_page, playlistId=id
        )

        response = request.execute()

        results_per_page = response["pageInfo"]["resultsPerPage"]
        current_items = current_items + results_per_page

        next_page = self.convert_to_playlist_items(playlist, response)
        return current_items, next_page

    def initial_list_playlist_items(self, playlist: Playlist, current_items: int):
        """
        Created the initial request to list the items in the playlist

        Args:
            playlist (Playlist): Playlist object
            current_items (int): Number of items currently processed

        Returns:
            _type_: _description_
        """
        id = playlist.id
        request = self.youtube.playlistItems().list(part="snippet", playlistId=id)

        response = request.execute()
        total_results = response["pageInfo"]["totalResults"]
        results_per_page = response["pageInfo"]["resultsPerPage"]
        current_items = current_items + results_per_page

        next_page = self.convert_to_playlist_items(playlist, response)
        return current_items, total_results, next_page

    def convert_to_playlist_items(
        self, playlist: Playlist, response: dict
    ) -> Optional[str]:
        """
        Converst the raw api output to Playlist format

        Args:
            playlist (Playlist): Playlist object
            response (dict): raw response

        Returns:
            Optional[str]: Next page token or None
        """
        items = response["items"]
        for item in items:
            playlist_item = self._convert_to_PlaylistItem(item)
            playlist.add_item(playlist_item)
        if "nextPageToken" in response:
            next_page = response["nextPageToken"]
            return next_page
        return None

    def _convert_to_PlaylistItem(self, item: dict) -> PlayListItem:
        """
        Convert the raw API output to PlaylistItems

        Args:
            item (dict): json output of the response

        Returns:
            PlayListItem: PlayListItem object
        """
        id = item["id"]
        videoId = item["snippet"]["resourceId"]["videoId"]
        publishedAt = item["snippet"]["publishedAt"]
        channelId = item["snippet"]["channelId"]
        description = item["snippet"]["description"]
        title = item["snippet"]["title"]
        playlist_item = PlayListItem(
            id, title, publishedAt, channelId, description, videoId
        )

        return playlist_item

    def populate_new_playlist(self, playlist: Playlist) -> None:
        """
        Populates the new playlist with videos from the chosen playlist

        Args:
            playlist (Playlist): Playlist object
        """
        random.shuffle(playlist.items)

        body = {}
        i = 0
        playlistId = self.playlists[-1].id

        for item in playlist.items:
            entry = {
                "snippet": {
                    "playlistId": playlistId,
                    "position": i,
                    "resourceId": {"kind": "youtube#video", "videoId": item.videoId},
                }
            }
            body.update(entry)

            request = self.youtube.playlistItems().insert(part="snippet", body=body)
            try:
                response = request.execute()
                _logger.debug(response)
            except HttpError as error:
                _logger.warning(
                    "Error in populating new playlist status code: %s details: %s",
                    error.status_code,
                    error.error_details,
                )
            i += 1
            # limit request/s somewhat
            time.sleep(0.1)

            if i == self.chunks:
                # sleep for 25 hours, youtube playlist inserts are limited in requests/day
                time.sleep(90000)

    def choose_playlist(self) -> Playlist:
        """
        Lets the user choose a playlist to randomize

        Returns:
            Playlist: Playlist object
        """

        titles = [playlist.title for playlist in self.playlists]
        playlist_title = prompt(
            "Youtube playlist to use ", completer=WordCompleter(titles)
        ).strip()
        self._create_new_playlist(playlist_title)
        playlist = self.get_id(playlist_title)
        return playlist

    def _create_new_playlist(self, title: str) -> None:
        """
        Creates a new playlist based on the provided title

        Args:
            title (str): Title to use
        """
        playlist_title = prompt(
            "Youtube playlist to use ", default="{}-randomized".format(title)
        )

        playlist_id = self.create_youtube_playlist(playlist_title)
        new_playlist = Playlist(playlist_id, playlist_title)
        self.playlists.append(new_playlist)

    def get_id(self, title: str) -> Playlist:
        """
        Gets the id of a playlist by title

        Args:
            title (str): Title to search for

        Returns:
            Playlist: Playlist object
        """
        for playlist in self.playlists:
            if playlist.title == title:
                return playlist
