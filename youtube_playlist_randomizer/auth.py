"""
Authenticate the users to youtube.
"""

import os

import google_auth_oauthlib.flow
import googleapiclient.discovery
from google_auth_oauthlib.flow import InstalledAppFlow


scopes = ["https://www.googleapis.com/auth/youtube.force-ssl"]


def auth(file):
    """
    Authenticates the user to access the users youtube content
    :param args: sys arguments
    :return: youtube api client
    """
    # Disable OAuthlib's HTTPS verification when running locally.
    # *DO NOT* leave this option enabled in production.
    os.environ["OAUTHLIB_INSECURE_TRANSPORT"] = "1"

    api_service_name = "youtube"
    api_version = "v3"
    client_secrets_file = file

    # Get credentials and create an API client
    flow = InstalledAppFlow.from_client_secrets_file(client_secrets_file, scopes)
    try:
        credentials = flow.run_local_server()
    except google_auth_oauthlib.flow.InstalledAppFlowError as e:
        print(f"Error during authentication: {e}, trying console flow.")
        credentials = flow.run_console()
    youtube = googleapiclient.discovery.build(
        api_service_name, api_version, credentials=credentials
    )
    return youtube
