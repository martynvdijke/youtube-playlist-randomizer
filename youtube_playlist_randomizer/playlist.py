"""
File where all the magic happens
"""

import logging
import random
from googleapiclient.errors import HttpError
from prompt_toolkit import prompt
from prompt_toolkit.completion import WordCompleter

_logger = logging.getLogger(__name__)

# pylint: disable=C0200


class PlayListRandomizer:
    """
    Main playlist randomizer class to easily hold all data required
    """

    def __init__(self, youtube):
        """

        :param youtube: authorized youtube entity
        """
        self.youtube = youtube
        self.playlist_id = None
        self.new_playlist_id = None
        # get the users playlist and fires the main scripts
        self.get_playlists()

    def get_playlists(self):
        """
        Gets all of the users playlists
        :return: call to cli input for choosing the playlist
        """
        current_items = 0
        # do the initial request

        request = self.youtube.playlists().list(part="snippet", mine=True)
        response = request.execute()
        title_list = []
        id_list = []
        _logger.debug(response)
        # add all titles to the title list along with the id list
        for item in response["items"]:
            title_list.append(item["snippet"]["localized"]["title"])
            id_list.append(item["id"])

        # get the info to loop over all playlist
        total_items = response["pageInfo"]["totalResults"]
        results_per_page = response["pageInfo"]["resultsPerPage"]
        current_items = current_items + results_per_page
        next_page = response["nextPageToken"]

        # loop over all pages containing results
        for i in range(current_items, total_items, results_per_page):
            _logger.debug("Loop %d", i)
            request = self.youtube.playlistItems().list(
                part="snippet", mine=True, pageToken=next_page
            )
            response = request.execute()

            for item in response["items"]:
                title_list.append(item["snippet"]["localized"]["title"])
                id_list.append(item["id"])
            try:
                next_page = response["nextPageToken"]
            except HttpError:
                _logger.debug("Reached end of the playlists ")
                break

        _logger.debug("Number of items in video list %d", len(title_list))
        self.choose_playlist(title_list, id_list)

    def make_new_playlist(self, title):
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
            self.new_playlist_id = response["id"]
        except HttpError as error:
            _logger.warning(
                "Error in making new playlist status code: %s details: %s",
                error.status_code,
                error.error_details,
            )

    def get_items_in_playlist(self):
        """
        Get all the video id items in a playlist
        :return:
        """
        current_items = 0
        videoid_list = []

        # get inital output of playlist
        request = self.youtube.playlistItems().list(
            part="snippet", playlistId=self.playlist_id
        )
        response = request.execute()
        # get all the info about the number of pages containing results
        total_items = response["pageInfo"]["totalResults"]
        results_per_page = response["pageInfo"]["resultsPerPage"]
        current_items = current_items + results_per_page
        next_page = response["nextPageToken"]

        for item in response["items"]:
            videoid_list.append(item["snippet"]["resourceId"]["videoId"])

        for i in range(current_items, total_items, results_per_page):
            _logger.debug("Loop %d", i)
            request = self.youtube.playlistItems().list(
                part="snippet", pageToken=next_page, playlistId=self.playlist_id
            )
            response = request.execute()

            for item in response["items"]:
                videoid_list.append(item["snippet"]["resourceId"]["videoId"])

            try:
                next_page = response["nextPageToken"]
            except HttpError:
                _logger.debug("Reached end of the playlist")
                break
        _logger.debug("Number of items in video list %d", len(videoid_list))
        # populate the new playlist with all the video_id's
        self.populate_new_playlist(videoid_list)

    def populate_new_playlist(self, videoid_list):
        """
        Populates the new playlist with a shuffles version of the original playlist
        :param videoid_list: list containting all the video id's
        :return:
        """
        # shuffles playlist
        random.shuffle(videoid_list)

        body = {}
        # loop over all values and add them to the request
        for i in range(0, len(videoid_list)):
            entry = {
                "snippet": {
                    "playlistId": self.new_playlist_id,
                    "position": i,
                    "resourceId": {"kind": "youtube#video", "videoId": videoid_list[i]},
                }
            }

            body.update(entry)

        # send the request
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

    def choose_playlist(self, title_list, id_list):
        """
        Lets the user choose which playlist needs to be shuffled
        :param title_list: list of titles
        :param id_list: list of playlist id's
        :return:
        """
        playlist_title = prompt(
            "Youtube playlist to use ", completer=WordCompleter(title_list)
        ).strip()
        new_playlist_title = prompt(
            "Youtube playlist to use ", default="{}-randomized".format(playlist_title)
        )
        index = title_list.index(playlist_title)
        playlist_id = id_list[index]
        self.playlist_id = playlist_id

        self.make_new_playlist(new_playlist_title)
        _logger.debug(
            "Getting items of playlist %s with playlist id %s",
            playlist_title,
            playlist_id,
        )
        self.get_items_in_playlist()
