class PlayListItem:
    """
    PlayListItem object
    """

    def __init__(
        self,
        id: str,
        title: str,
        publishedAt: str,
        channelId: str,
        description: str,
        videoId: str,
    ) -> None:
        """
        PlayListItem object

        Args:
            id (str): playlist object
            title (str): title
            publishedAt (str): publishedAt
            channelId (str): channelId
            description (str): description
            videoId (str): videoId
        """
        self.id = id
        self.title = title
        self.publishedAt = publishedAt
        self.channelId = channelId
        self.description = description
        self.videoId = videoId
