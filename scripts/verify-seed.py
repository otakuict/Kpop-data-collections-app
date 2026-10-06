"""Verify the actual checked-in seed against every source CSV row."""
import csv
import hashlib
import json
import sqlite3
import sys
from pathlib import Path

root = Path(__file__).resolve().parents[1]
source_path = Path(sys.argv[1]) if len(sys.argv) > 1 else root / 'data/source.csv'
database_path = Path(sys.argv[2]) if len(sys.argv) > 2 else root / 'data/seed.sqlite'
with source_path.open(newline='') as source:
    rows = list(csv.reader(source))
header_index = next(i for i, row in enumerate(rows) if row[:3] == ['Date', 'Name', 'GROUP'])
source_rows = [row for row in rows[header_index + 1:] if ''.join(row).strip()]
db = sqlite3.connect(f'file:{database_path}?mode=ro', uri=True)
assert db.execute('PRAGMA integrity_check').fetchone()[0] == 'ok'
assert not db.execute('PRAGMA foreign_key_check').fetchall()
stored = db.execute('SELECT title,group_name,raw_date,source,example,raw_cells,date FROM sets ORDER BY id').fetchall()
assert len(stored) == len(source_rows) and source_rows
for actual, row in zip(stored, source_rows, strict=True):
    assert actual[:5] == (row[1].strip(), row[2].strip(), row[0].strip(), row[3].strip(), row[5].strip())
    raw = json.loads(actual[5])
    assert raw['cells'] == row and raw['header'] == rows[header_index]
    if row[0] in ('', '0'):
        assert actual[6] == ''
images = db.execute('SELECT bytes,hash,mime FROM images').fetchall()
assert images, 'Seed must include actual extracted image BLOBs'
for blob, digest, mime in images:
    assert mime == 'image/jpeg' and blob.startswith(b'\xff\xd8')
    assert hashlib.sha256(blob).hexdigest() == digest
counts = dict(db.execute('SELECT n,count(*) FROM (SELECT count(images.id) n FROM sets LEFT JOIN images ON sets.id=images.set_id GROUP BY sets.id) GROUP BY n'))
assert all(0 <= count <= 5 for count in counts), f'Set exceeds image limit: {counts}'
states = dict(db.execute('SELECT image_state,count(*) FROM sets GROUP BY image_state'))
assert sum(states.values()) == len(source_rows)
print(f'Seed verified: {len(stored)} sets, {len(images)} stored previews, states={states}, image_counts={counts}')
