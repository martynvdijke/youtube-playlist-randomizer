
from youtube_playlist_randomizer.playlistitem import PlayListItem
class Playlist:
    """
    Playlist DTO
    """
    
    def __init__(self, id:str, title:str):
        """
        PLaylist object

        Args:
            id (str): playlist id
            title (str): playlist title
        """
        self.id = id
        self.title = title
        self.items = []
    
    def add_item(self, item: PlayListItem) -> None:
        """
        Adds a playlist item to the playlist

        Args:
            item (PlayListItem): Playlist Object
        """
        self.items.append(item)