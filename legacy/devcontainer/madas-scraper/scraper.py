import requests
from bs4 import BeautifulSoup

url = "https://www.madas.it/es/inicio"
print(f"Scraping: {url}")

r = requests.get(url, timeout=30, headers={"User-Agent": "Mozilla/5.0"})
soup = BeautifulSoup(r.text, "lxml")

print(f"Titulo: {soup.title.string}")

links = soup.find_all("a", href=True)
print(f"Links encontrados: {len(links)}")

for a in links[:15]:
    href = a.get("href", "")
    text = a.get_text(strip=True)[:40]
    if text:
        print(f"  {text}: {href}")
