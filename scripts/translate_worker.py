#!/usr/bin/env python3
"""
Translation worker script for Iskoces.
This script runs as a subprocess and translates text using Argos Translate directly.
It listens on a Unix domain socket for requests and responds via the same socket.

This eliminates HTTP overhead and allows fast local communication.
"""

import sys
import json
import socket
import os
import argostranslate.package
import argostranslate.translate

def translate_text(text, source_lang, target_lang):
    """Translate text using Argos Translate library directly."""
    try:
        # Ensure packages are installed
        argostranslate.package.update_package_index()
        available_packages = argostranslate.package.get_available_packages()
        
        # Find and install the required translation package if needed
        package_to_install = next(
            (pkg for pkg in available_packages 
             if pkg.from_code == source_lang and pkg.to_code == target_lang),
            None
        )
        if package_to_install and not package_to_install.installed:
            argostranslate.package.install_from_path(package_to_install.download())
        
        # Translate directly using the library
        translated = argostranslate.translate.translate(text, source_lang, target_lang)
        return translated
    except Exception as e:
        raise Exception(f"Translation failed: {str(e)}")

def handle_request(conn):
    """Handle a single translation request."""
    try:
        # Read request (JSON line)
        data = conn.recv(4096)
        if not data:
            return False
        
        # Parse request
        request = json.loads(data.decode('utf-8'))
        text = request.get('text', '')
        source_lang = request.get('source_lang', 'en')
        target_lang = request.get('target_lang', 'fr')
        
        # Translate
        translated = translate_text(text, source_lang, target_lang)
        
        # Send response
        response = {
            'success': True,
            'translated_text': translated
        }
        response_json = json.dumps(response) + '\n'
        conn.sendall(response_json.encode('utf-8'))
        
        return True
        
    except json.JSONDecodeError as e:
        error_response = {
            'success': False,
            'error': f'Invalid JSON: {str(e)}'
        }
        conn.sendall((json.dumps(error_response) + '\n').encode('utf-8'))
        return False
    except Exception as e:
        error_response = {
            'success': False,
            'error': str(e)
        }
        conn.sendall((json.dumps(error_response) + '\n').encode('utf-8'))
        return False

def main():
    """Main loop: listen on Unix socket, handle requests."""
    if len(sys.argv) < 3 or sys.argv[1] != '--socket':
        print("Usage: translate_worker.py --socket /path/to/socket", file=sys.stderr)
        sys.exit(1)
    
    socket_path = sys.argv[2]
    
    # Remove old socket if it exists
    if os.path.exists(socket_path):
        os.remove(socket_path)
    
    # Create Unix domain socket server
    sock = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
    sock.bind(socket_path)
    sock.listen(5)
    
    # Make socket readable/writable by group (for Kubernetes)
    os.chmod(socket_path, 0660)
    
    print(f"Worker listening on {socket_path}", file=sys.stderr, flush=True)
    
    # Accept connections and handle requests
    while True:
        try:
            conn, addr = sock.accept()
            # Handle request (blocking)
            handle_request(conn)
            conn.close()
        except KeyboardInterrupt:
            break
        except Exception as e:
            print(f"Error handling request: {e}", file=sys.stderr, flush=True)
            if 'conn' in locals():
                conn.close()
    
    sock.close()
    os.remove(socket_path)

if __name__ == '__main__':
    main()

