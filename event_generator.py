import json
from time import time
from urllib import request

API_URL = "http://localhost:8080/events"

# manual event generation for testing purposes
event = {
    "event_id": "event-001",
    "player_id": "player-1",
    "event_type": "login",
    "timestamp": int(time.time())
}

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