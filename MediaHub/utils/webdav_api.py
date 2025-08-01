import os
import json
import time
import socket
import requests
from .logging_utils import log_message
from MediaHub.config.config import get_cinesync_ip, get_cinesync_api_port
from MediaHub.utils.dashboard_utils import is_dashboard_available, send_dashboard_notification

class ConnectionManager:
    """Simple connection manager with retry capability."""

    def __init__(self):
        self._session = None
        self._failures = 0

    def get_session(self):
        """Get or create a session."""
        if self._session is None or self._failures > 3:
            if self._session:
                self._session.close()
            self._session = requests.Session()
            self._failures = 0
        return self._session

    def mark_failure(self):
        """Mark a connection failure."""
        self._failures += 1

    def mark_success(self):
        """Mark a successful connection."""
        self._failures = 0

# Global connection manager instance
_connection_manager = ConnectionManager()

def is_server_available(host, port, timeout=0.1):
    """Quick check if server is available on the given host and port."""
    try:
        with socket.create_connection((host, int(port)), timeout=timeout):
            return True
    except (socket.error, socket.timeout):
        return False



def send_structured_message_http(message_type, data, max_retries=2):
    """Send structured message via HTTP to WebDavHub API with retry."""
    host = get_cinesync_ip()
    port = get_cinesync_api_port()
    api_url = f"http://{host}:{port}/api/mediahub/message"

    structured_msg = {
        "type": message_type,
        "timestamp": time.time(),
        "data": data
    }

    for attempt in range(max_retries + 1):
        try:
            session = _connection_manager.get_session()
            response = session.post(api_url, json=structured_msg, timeout=5)

            if response.status_code == 200:
                _connection_manager.mark_success()
                return True
            elif response.status_code >= 500 and attempt < max_retries:
                _connection_manager.mark_failure()
                time.sleep(1)
                continue
            return False

        except (requests.exceptions.ConnectionError, BrokenPipeError) as e:
            if "broken pipe" in str(e).lower():
                log_message(f"Broken pipe sending {message_type}, attempt {attempt + 1}", level="DEBUG")

            if attempt < max_retries:
                _connection_manager.mark_failure()
                time.sleep(1)
                continue
            return False

        except Exception as e:
            log_message(f"Error sending {message_type}: {e}", level="DEBUG")
            if attempt < max_retries:
                time.sleep(1)
                continue
            return False

    return False

def send_structured_message(message_type, data, max_retries=2):
    """Send structured message to WebDavHub API with retry logic."""
    try:
        if not is_dashboard_available():
            return False
        return send_structured_message_http(message_type, data, max_retries)
    except Exception as e:
        log_message(f"Error sending structured message: {e}", level="DEBUG")
        return False
