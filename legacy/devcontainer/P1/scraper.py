#!/usr/bin/env python3
"""
Web Scraper for content.helpme-codesys.com
Creado por Claude Code via instrucciones
"""
import os
import time
import json
import requests
from urllib.parse import urljoin, urlparse
from bs4 import BeautifulSoup
from pathlib import Path

BASE_URL = "https://content.helpme-codesys.com/"
OUTPUT_DIR = Path("./web")
VISITED_FILE = Path("visited.json")
DELAY = 0.3

def load_visited():
    """Carga URLs ya visitadas para resume."""
    if VISITED_FILE.exists():
        return set(json.loads(VISITED_FILE.read_text()))
    return set()

def save_visited(visited):
    """Guarda URLs visitadas."""
    VISITED_FILE.write_text(json.dumps(list(visited)))

def url_to_path(url):
    """Convierte URL a path local."""
    parsed = urlparse(url)
    path = parsed.path or "/index.html"
    if path.endswith("/"):
        path += "index.html"
    return OUTPUT_DIR / path.lstrip("/")

def download_page(url, visited):
    """Descarga una página y retorna links encontrados."""
    if url in visited:
        return []
    
    visited.add(url)
    
    try:
        headers = {"User-Agent": "Mozilla/5.0 (compatible; Scraper/1.0)"}
        response = requests.get(url, headers=headers, timeout=30)
        if response.status_code != 200:
            print(f"  SKIP [{response.status_code}] {url[:50]}")
            return []
    except Exception as e:
        print(f"  ERROR: {url[:50]} - {e}")
        return []
    
    local_path = url_to_path(url)
    local_path.parent.mkdir(parents=True, exist_ok=True)
    
    links = []
    content_type = response.headers.get("content-type", "")
    
    if "html" in content_type:
        soup = BeautifulSoup(response.text, "html.parser")
        
        # Extract links from <a> tags
        for a in soup.find_all("a", href=True):
            href = urljoin(url, a["href"])
            if href.startswith(BASE_URL) and "#" not in href:
                links.append(href.split("#")[0])
        
        # Extract resources (CSS, JS, images)
        for tag in soup.find_all(["link", "script", "img"]):
            src = tag.get("href") or tag.get("src")
            if src:
                full_url = urljoin(url, src)
                if full_url.startswith(BASE_URL):
                    links.append(full_url)
        
        local_path.write_text(response.text, encoding="utf-8")
    else:
        local_path.write_bytes(response.content)
    
    print(f"[{len(visited):4d}] {url[:70]}...")
    save_visited(visited)
    time.sleep(DELAY)
    
    return links

def main():
    print(f"Iniciando mirror de {BASE_URL}")
    print(f"Output: {OUTPUT_DIR.absolute()}")
    print("-" * 60)
    
    OUTPUT_DIR.mkdir(exist_ok=True)
    visited = load_visited()
    
    if visited:
        print(f"Resumiendo desde {len(visited)} URLs ya descargadas")
    
    queue = [BASE_URL]
    
    while queue:
        url = queue.pop(0)
        links = download_page(url, visited)
        
        # Add new links to queue
        for link in links:
            if link not in visited and link not in queue:
                queue.append(link)
    
    print("-" * 60)
    print(f"Completado! {len(visited)} páginas descargadas")
    print(f"Archivos en: {OUTPUT_DIR.absolute()}")

if __name__ == "__main__":
    main()
