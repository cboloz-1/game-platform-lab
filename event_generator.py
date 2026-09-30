import json
from time import sleep, time
from urllib import request
from uuid import uuid4
from random import choice

API_URL = "http://localhost:8080/events"

EVENT_TYPES = [
    "match_start",
    "match_finished",
    "login",
    "logout",
]

PLAYER_IDS = [
    "player-1",
    "player-2",
    "player-3",
]
# creates a fresh event when this function is called
def create_event():
    return {
    "event_id": str(uuid4()), # generates a unique identifier for the event
    "player_id": choice(PLAYER_IDS), # randomly selects a player ID from the PLAYER_IDS list
    "event_type": choice(EVENT_TYPES), # randomly selects an event type from the EVENT_TYPES list
    "timestamp": int(time())
}

while True:
    event = create_event()

    # Converts 'event' dictionary to JSON and encodes it to bytes
    event_json = json.dumps(event).encode("utf-8")

    # Creates an HTTP POST request with the event JSON as the body and appropriate headers
    http_request = request.Request(
        API_URL,
        data=event_json,
        headers={"Content-Type": "application/json"},
        method="POST"
    )

    # request.urlopen() sends the HTTP request and returns a response object. 
    # The 'with' statement ensures that the response is properly closed after reading.
    with request.urlopen(http_request) as response:
        response_body = response.read().decode("utf-8")

        # Prints the HTTP status code and the response body
        print(f"Status: {response.status}")
        print(f"Response: {response_body}")

    sleep(2)  # Waits for 2 seconds before generating the next event