"""Refresh factual name/alias seed from profile pages for groups already in gallery.

Uses the stdlib only. No credentials, gallery content or images are sent upstream.
Each profile is recorded with provenance; failures do not overwrite the seed.
Run manually after reviewing source changes, not on application startup.
"""
import concurrent.futures
import argparse
import datetime
import html.parser
import json
import pathlib
import re
import sqlite3
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[1]
SLUGS = {
    'ARTMS': 'artms', 'BABYMONSTER': 'babymonster', 'BLACK PINK': 'black-pink',
    'CLASS:y': 'classy', 'FIFTY FIFTY': 'fifty-fifty', 'FROMIS_9': 'fromis_9',
    'H1key': 'h1-key', 'H2H': 'hearts2hearts', 'ICHILLIN': 'ichillin',
    'IDLE': 'idle', 'ILLIT': 'illit', 'ITZY': 'itzy', 'IVE': 'ive',
    'KISS OF LIFE': 'kiss-of-life', 'Kep1er': 'kep1er', 'KiiiKiii': 'kiiikiii',
    'LE SSERAFIM': 'le-sserafim', 'LIGHTSUM': 'lightsum', 'MEOVV': 'meovv',
    'NMIXX': 'nmixx', 'NewJeans': 'newjeans', 'OH MY GIRL': 'oh-my-girl',
    'RED VELVET': 'red-velvet', 'S2IT': 's2it', 'SNSD': 'girls-generation-snsd',
    'StayC': 'stayc', 'TWICE': 'twice', 'TripleS': 'triples', 'aespa': 'aespa',
    'cignature/latency': 'cignature',
}


class Text(html.parser.HTMLParser):
    def __init__(self):
        super().__init__()
        self.parts = []

    def handle_data(self, data):
        self.parts.append(data)

    def handle_starttag(self, tag, attrs):
        if tag in ('br', 'p', 'h2', 'h3', 'h4', 'li'):
            self.parts.append('\n')


def profile(pair):
    group, slug = pair
    url = slug if slug.startswith('https://') else f'https://kprofiles.com/{slug}-members-profile/' if group != 'NMIXX' else 'https://kprofiles.com/nmixx-profile/'
    request = urllib.request.Request(url, headers={'User-Agent': 'Mozilla/5.0'})
    raw = urllib.request.urlopen(request, timeout=30).read().decode()
    parser = Text()
    parser.feed(raw)
    text = ''.join(parser.parts)
    return group, parse_profile_names(text, url)


def parse_profile_names(text, url):
    # Extract only factual name fields, never biographies or images.
    names = list(re.finditer(r'((?:Stage|Full|Birth|Real|Korean)[^\n:]{0,40}Name)\s*:\s*([^\n]+)', text))
    starts = [m for i, m in enumerate(names) if m.group(1).startswith('Stage') or
              (m.group(1).startswith('Birth') and (i == 0 or m.start() - names[i - 1].end() > 500))]
    members = []
    for index, match in enumerate(starts):
        field = match.group(2).strip()
        english = re.split(r'\s*\(', field)[0].strip()
        if not english or not re.search('[A-Za-z]', english):
            continue
        end = starts[index + 1].start() if index + 1 < len(starts) else match.end() + 2500
        block = text[match.start():end]
        aliases = re.findall(r'[가-힣]+(?:\s+[가-힣]+)*', field)
        for birth in re.finditer(r'(?:Birth|Full|Real|Korean) Name\s*:\s*([^\n]+)', block):
            aliases += re.findall(r'[가-힣]+(?:\s+[가-힣]+)*', birth.group(1))
            latin = re.split(r'\s*\(', birth.group(1))[0].strip()
            if latin and re.fullmatch(r"[A-Za-z .’'\-]+", latin):
                aliases.append(latin)
        members.append({'name': english, 'aliases': sorted(set(aliases)), 'source': url})
    if not members:
        raise ValueError(f'No member names parsed: {url}')
    return members


if __name__ == '__main__':
    args = argparse.ArgumentParser()
    args.add_argument('--group', help='Refresh just one existing gallery category')
    selected = args.parse_args().group
    db = sqlite3.connect(f'file:{ROOT}/data/seed.sqlite?mode=ro', uri=True)
    groups = {row[0] for row in db.execute('SELECT DISTINCT group_name FROM sets')}
    db.close()
    results, failures = {}, {}
    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        futures = {pool.submit(profile, pair): pair[0] for pair in SLUGS.items() if pair[0] in groups and (not selected or pair[0] == selected)}
        for future in concurrent.futures.as_completed(futures):
            group = futures[future]
            try:
                _, results[group] = future.result()
                print(group, len(results[group]))
            except Exception as error:
                failures[group] = str(error)
                print('FAILED', group, error)
        if 'SOLO' in groups and (not selected or selected == 'SOLO'):
            solo = []
            for slug in ('kwon-eunbi-profile-facts', 'yena-ex-izone-profile', 'bibi-profile', 'dayoung-wjsn-profile'):
                try:
                    _, members = profile(('SOLO', f'https://kprofiles.com/{slug}/'))
                    solo.extend(members)
                except Exception as error:
                    failures[f'SOLO:{slug}'] = str(error)
            results['SOLO'] = solo
    # Reviewed stage-name correction: the scraped birth-name field currently
    # differs from the official Japanese member profile. Keep source provenance.
    for member in results.get('TripleS', []):
        if member['name'] == 'Kim Junseo':
            member.update(name='Kim Nakyoung', aliases=['김나경', '나경', 'Kim Na-kyoung', 'Nakyoung'], source='https://www.triples-official.jp/')
    target = ROOT / 'backend/internal/gallery/assets/artist-roster.json'
    if target.exists():
        previous = json.loads(target.read_text())
        if selected:
            results = {**previous['groups'], **results}
        for group in failures:
            if group in previous['groups']:
                results[group] = previous['groups'][group]
            if group.startswith('SOLO:'):
                failed_url = f"https://kprofiles.com/{group.split(':', 1)[1]}/"
                results.setdefault('SOLO', []).extend(m for m in previous['groups'].get('SOLO', []) if m['source'] == failed_url)
    target.write_text(json.dumps({'checkedOn': datetime.datetime.now(datetime.timezone.utc).date().isoformat(), 'groups': dict(sorted(results.items())), 'failures': failures}, ensure_ascii=False, indent=2) + '\n')
