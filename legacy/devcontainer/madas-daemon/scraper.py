import requests
from bs4 import BeautifulSoup
import time
from datetime import datetime

URL = "https://www.madas.it/es/inicio"

def scrape_madas():
    try:
        print(f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] Scraping {URL}...")

        headers = {
            'User-Agent': 'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36'
        }

        response = requests.get(URL, headers=headers, timeout=10)
        response.raise_for_status()

        soup = BeautifulSoup(response.content, 'lxml')

        # Extraer título de la página
        title = soup.find('title')
        title_text = title.get_text(strip=True) if title else "No title found"

        print(f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] Success! Title: {title_text}")
        print(f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] Page size: {len(response.content)} bytes")

    except Exception as e:
        print(f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] Error: {str(e)}")

if __name__ == "__main__":
    print("Madas Scraper Daemon started!")
    print(f"Target URL: {URL}")
    print(f"Interval: 30 seconds")
    print("-" * 60)

    while True:
        scrape_madas()
        print(f"[{datetime.now().strftime('%Y-%m-%d %H:%M:%S')}] Sleeping for 30 seconds...")
        print("-" * 60)
        time.sleep(30)
