# BIAS — K-pop image archive

เว็บ gallery พร้อม API และ back-office ใน monorepo เดียว ใช้ Svelte 5 + Vite, Go/Gin, SQLite และ Docker โดย SQLite เก็บทั้ง metadata และรูป preview แบบ BLOB

## พัฒนาด้วย Docker (live reload)

```sh
./scripts/init-local.sh
docker compose up -d --build
```

`compose.yaml` เป็นโหมดพัฒนา: bind mount โค้ดของทั้ง 3 ส่วนจากเครื่องเข้า container โดย frontend/back-office ใช้ Vite HMR และ backend ใช้ [Air](https://github.com/air-verse/air) rebuild/restart อัตโนมัติเมื่อแก้ `.go`, `go.mod`, `go.sum` มี polling สำหรับ file changes บน macOS/Docker

หลังเปิดครั้งแรก แก้โค้ดแล้วเห็นผลได้เลย ไม่ต้อง build Docker image ใหม่ Dependencies ของ Node และ Go อยู่ใน volumes แยกจาก `node_modules` บนเครื่อง ถ้าเปลี่ยน `package.json` หรือ lockfile ให้ใช้ `docker compose restart frontend backoffice` เพื่อติดตั้ง dependencies ใหม่

Build แบบ nginx/production ใช้:

```sh
docker compose -f compose.build.yaml up -d --build
```

ถ้าเครื่องมี Compose แบบ standalone ให้ใช้ `docker-compose` แทน `docker compose` ทุกคำสั่ง

- Gallery: http://localhost:5173
- Back-office: http://localhost:5174
- API health: http://localhost:8080/api/health

เข้า back-office ด้วยค่า `ADMIN_TOKEN` ใน `.env` ซึ่งสคริปต์สร้างแบบสุ่มให้ ค่านี้ใช้เฉพาะฝั่ง server และเก็บใน memory ของ browser หลัง sign in; refresh/ปิดแท็บแล้วต้อง sign in ใหม่ `.env` ไม่ถูก commit หรือใส่ Docker image

API เป็นผู้เขียน SQLite เพียง service เดียว ข้อมูลอยู่ใน named volume `gallery-data` และคงอยู่หลัง restart/rebuild ใช้ `docker compose down` เพื่อหยุดโดยเก็บข้อมูลไว้

## Gallery และ back-office

Gallery อ่านข้อมูลจาก API และรองรับค้นหาชื่อ/กลุ่ม/source, filter กลุ่มและช่วงวันที่, เรียงล่าสุด/เก่าสุด/ชื่อ, group ตามศิลปินหรือวันที่, pagination, carousel บนแต่ละ card (แถบกึ่งกลางด้านล่างรูป กดค้างแล้วลากซ้าย/ขวาด้วยเมาส์หรือ touch; รองรับ ArrowLeft/ArrowRight และ Home/End เมื่อ focus ที่แถบ) และ dialog รายละเอียดพร้อมรูปและลิงก์ example post Filter อยู่ใน URL เพื่อแชร์มุมมองได้

Back-office เพิ่มและแก้ไข set ด้วยมือ, แสดง `set_id` ในตารางและ editor, อัปโหลด JPEG/PNG/GIF/WebP ให้แต่ละชุดมี 2–5 รูปที่ไม่ซ้ำกัน (ไม่เกิน 12 MiB/รูป), import Google Sheet ที่อ่านได้โดยไม่ต้อง sign in หรือ CSV export และสั่ง extract/retry preview ที่ยังขาดได้ รูปจะตรวจชนิดและจำนวน pixel แล้วปรับขนาดไม่เกิน 1200 pixels ต่อด้านก่อนเก็บเป็น JPEG ใน SQLite

ชุดที่นำเข้าหรือชุดเดิมซึ่งยังมี 0–1 รูปจะคง metadata ไว้และแสดง `needs 2+` จนเติมรูปได้ครบ API จำกัดสูงสุด 5 รูปต่อชุดทั้งจาก upload และ extraction การอัปโหลดที่ทำให้เกิน 5 หรือยังมีไม่ถึง 2 รูปจะยกเลิกทั้ง batch โดยตรวจจาก hash ของรูปจริง

CSV template ดาวน์โหลดได้จากหน้า Import & previews หรือไฟล์ `backoffice/public/template.csv`:

```csv
ID,Date,Name,GROUP,Source,Example,REMARK
set-001,260915,260915 Karina,aespa,kdatastudio,https://ganknow.com/post/5ddbab87-f0f2-404a-89a9-eb2f897aad61,Example
```

`ID` ไม่บังคับ แต่ช่วยให้เปลี่ยนชื่อ/กลุ่ม/วันที่แล้วอัปเดต record เดิมได้ ถ้าไม่มี ID ใช้ source sheet + กลุ่ม + วันที่ดิบ + ชื่อเป็น identity พร้อม occurrence สำหรับแถวซ้ำ วันที่รองรับ YYMMDD หรือ YYYY-MM-DD; blank/0 เป็น unknown และวันที่ผิดจะมี warning ทุกคอลัมน์ต้นฉบับถูกเก็บไว้

Import ซ้ำไม่เพิ่ม record ซ้ำใน identity เดิม อัปเดต metadata ของแถวที่ตรงกันและคง manual records ไว้ ไม่ลบ record ที่หายไปจาก Sheet เปลี่ยน Example แล้วล้าง preview เดิมเพื่อไม่แสดงรูปผิดชุด การแก้ record ที่มาจาก Sheet ด้วยมืออาจถูก import ครั้งถัดไปเขียนทับ; ใช้ Sheet เป็นต้นฉบับสำหรับ record นั้น

## ข้อมูลเริ่มต้น

ใช้ข้อมูลทั้งหมดจาก [Google Sheet ที่ให้มา](https://docs.google.com/spreadsheets/d/1z45mKRtLvTZth8b-uhqkMyjzxVtHI8b7YVcySQVeEkg/edit?gid=0) export เมื่อ 6 ตุลาคม 2026 โดยไม่ใช้ filtered view ที่ติดมากับ URL:

- `data/source.csv`: snapshot ต้นฉบับ 165 data rows, 31 กลุ่ม หัวตารางจริงอยู่แถวที่ 4
- `data/seed.sqlite`: metadata ทุกแถวและ preview จริงที่ดาวน์โหลดได้ ไม่ต้องต่อ Google ตอนเปิดระบบครั้งแรก
- seed ล่าสุด: 63 รูปจริง, 28 ชุดมีครบ 2–5 รูป (26 ชุดมี 2 รูป, 2 ชุดมี 5 รูป), 1 ชุดมีเพียง 1 รูป และ 136 ชุดยังไม่มีรูป โดย 37 แถวไม่มี public example URL
- รายงานการเติมรูปและเวลาพัก Gank: `docs/IMAGE_IMPORT_REPORT.json`

เก็บแถวที่ไม่มีรูปไว้ครบโดยแสดง “Preview unavailable” ไม่สร้างรูปสมมติ ตัวดึงรูปอ่าน Open Graph/twitter image และ URL ของ post media ใน Nuxt HTML ของ Gank หรือ product covers ในข้อมูล Inertia ของ Gumroad (อ่านเป็นข้อมูล ไม่ execute JavaScript) และตัด suffix social-card crop ของ Gank ที่พบว่า 404 เพื่อใช้ URL ต้นฉบับ เก็บลิงก์รูปต้นทางไว้กับแต่ละ BLOB

ต้นทางอาจลบโพสต์, ต้อง sign in หรือ rate limit ทำให้จำนวน preview ที่ดึงได้เปลี่ยนแปลงได้ มีการเว้นระยะคำขอต่อ host และแสดงเหตุผลที่ back-office; อัปโหลดรูปเองหรือ retry เมื่อโพสต์กลับมาอ่านได้

Refresh seed แบบ offline สำหรับ release ใหม่ (ต้องมี Go 1.25+ และ network):

```sh
./scripts/refresh-seed.sh
python3 scripts/verify-seed.py
```

Seed copy เกิดเฉพาะตอนยังไม่มี runtime database; refresh seed ไม่เขียนทับ volume ของระบบที่ใช้งานอยู่ ให้ import ผ่าน back-office เพื่ออัปเดต runtime data

## ดึงรูปเพิ่มและ Gank rate limit

```sh
# ใช้ API ของ stack ที่เปิดอยู่ อ่าน token จาก .env โดยไม่แสดง token ใน log
node scripts/fill-images.mjs --target=2
# เติมเฉพาะชุดให้มากขึ้น โดยไม่เกิน 5 รูป
node scripts/fill-images.mjs --set-id=1 --target=5
# บันทึกข้อมูลและรูปที่เติมแล้วเป็น seed สำหรับการติดตั้งใหม่
./scripts/export-seed.sh
```

ตัวดึงทำคำขอ Gank ทีละคำขอและเว้นอย่างน้อย 20 วินาทีหลังคำขอก่อนหน้าจบ โดย `ganknow.com` และทุก subdomain ใช้งบเดียวกัน จำกัดฝั่งแอป 60 คำขอต่อช่วง 24 ชั่วโมง ใช้ SQLite เก็บเวลารอ, lease, request count และ cooldown จึงไม่ reset เมื่อ Air/Docker restart นี่เป็นค่าที่แอปเลือกอย่างระมัดระวัง ไม่ใช่ quota ที่ Gank ประกาศ และไม่รับประกันว่าจะไม่ถูกบล็อก

เมื่อเจอ HTTP 429 จะพักอย่างน้อย 1 ชั่วโมง; HTTP 403 หรือ Cloudflare challenge จะพักอย่างน้อย 24 ชั่วโมง ถ้า `Retry-After` นานกว่านั้นจะใช้เวลาของ source ทั้ง back-office และ CLI หยุด batch ทันที ไม่ retry อัตโนมัติขณะพัก พฤติกรรม `Retry-After` อ้างอิง [Cloudflare HTTP 429 documentation](https://developers.cloudflare.com/support/troubleshooting/http-status-codes/4xx-client-error/error-429/)

HTML ของโพสต์ cache ไว้ 7 วัน และไม่ดาวน์โหลด URL รูปที่มีอยู่แล้วในชุด การ browse รูปทั้งหมดอ่านจาก SQLite จึงไม่สร้าง traffic ไป Gank หากโพสต์ไม่มีรูปสาธารณะครบ 2 รูป ต้องเติมด้วย upload หรือเปลี่ยน example link ใน back-office

`GET /api/admin/ingestion` แสดง policy, งบคำขอที่เหลือและเวลาพัก `scripts/refresh-seed.sh` รักษารูปเดิมและอัปเดต metadata เท่านั้น การดึงรูปใหม่ต้องผ่าน API เพื่อใช้นโยบาย rate limit ของ volume เดิม Script เขียนรายงานเริ่มต้นที่ `/tmp/bias-image-fill-report.json`; ใช้ `--report=<path>` เปลี่ยนตำแหน่งได้

## โครงสร้างและ API

```text
frontend/                 Svelte gallery + nginx + Dockerfile
backoffice/               Svelte administration + template + nginx + Dockerfile
backend/cmd/server/       Configuration, seed CLI, graceful shutdown
backend/internal/gallery/ Metadata/store, import, safe media fetch, Gin routes
data/                     Source snapshot + SQLite seed
.github/workflows/        FE, BE, back-office และ browser integration
docs/design/              สอง design candidates, cross-judge และแบบที่เลือก
```

| Endpoint | ใช้งาน |
|---|---|
| `GET /api/sets?q=&group=&from=&to=&sort=&page=&limit=` | รายการและ pagination; limit 1–200 |
| `GET /api/sets/:id` | รายละเอียด set และ local image URLs |
| `GET /api/facets` | กลุ่มและจำนวนชุด/preview |
| `GET /api/images/:id` | JPEG BLOB จาก SQLite |
| `GET /api/health` | Readiness ของ API + database |
| `GET /api/admin/ingestion` | Gank policy, remaining budget and cooldown |
| `GET /api/admin/status` | ตรวจ bearer token |
| `POST /api/admin/sets` / `PUT /api/admin/sets/:id` | เพิ่ม/แก้ไข metadata JSON |
| `POST /api/admin/sets/:id/images` | Multipart field `images` |
| `POST /api/admin/sets/:id/extract` | Extract images up to 5 (`?target=2` for bulk filling) |
| `POST /api/admin/import` | JSON `sheetUrl` หรือ multipart `csv` + `sheetUrl` |

Admin endpoints ใช้ `Authorization: Bearer <ADMIN_TOKEN>` Server ปฏิเสธ token ที่สั้นกว่า 24 ตัวอักษร Remote fetch รับเฉพาะ public HTTPS, ตรวจ DNS/IP และ redirect, ปฏิเสธ private/loopback/link-local/reserved addresses, จำกัดขนาด/เวลา/MIME/pixel Gallery และ back-office proxy `/api` ผ่าน Vite ในโหมดพัฒนาและ nginx ใน production จึงใช้ same-origin requests

## Local development โดยไม่ใช้ Docker

ต้องมี Node 24+, Go 1.25+ และ C compiler สำหรับ SQLite CGO

```sh
npm ci
./scripts/init-local.sh
# อ่าน ADMIN_TOKEN จาก .env แล้ว export ให้ process นี้
export ADMIN_TOKEN='<value from .env>'
# terminal 1, project root
(cd backend && DATABASE_PATH=../data/gallery.sqlite SEED_DATABASE=../data/seed.sqlite go run ./cmd/server)
# terminal 2
npm run dev --workspace frontend -- --port 5173
# terminal 3
npm run dev --workspace backoffice -- --port 5174
```

`API_PROXY` ปรับ target ของ Vite proxy ได้ ค่าเริ่มต้น `http://127.0.0.1:8080` ส่วนลิงก์ข้ามสอง apps ตั้งด้วย `VITE_BACKOFFICE_URL` และ `VITE_GALLERY_URL` ตอน build

## Verification

```sh
npm run check
npm test
npm run build
(cd backend && go test -race ./... && go vet ./...)
python3 scripts/verify-seed.py
```

Tests ตรวจ source fidelity/unknown dates/duplicate import/transaction rollback, image invalidation, safe URL/IP policy, admin auth, image upload+SQLite readback, gallery grouping และ timezone-independent dates

Browser acceptance ใช้ Compose อีก project เพื่อไม่แก้ข้อมูลจริง:

```sh
ADMIN_TOKEN=e2e-only-administrator-token-24chars GALLERY_PORT=5183 BACKOFFICE_PORT=5184 API_PORT=8180 \
  docker compose -f compose.build.yaml -p bias-archive-test up -d --build --wait
npx playwright install chromium
npx playwright test
# ล้างเฉพาะ volume ของ test project
ADMIN_TOKEN=e2e-only-administrator-token-24chars GALLERY_PORT=5183 BACKOFFICE_PORT=5184 API_PORT=8180 \
  docker compose -f compose.build.yaml -p bias-archive-test down -v
```

บนเครื่องที่มี Chrome อยู่แล้วใช้ `PLAYWRIGHT_CHROME_PATH='/Applications/Google Chrome.app/Contents/MacOS/Google Chrome' npx playwright test` ได้ Tests เปิด gallery filter/group/detail/empty state และ back-office login/manual upload/CSV import ทั้ง desktop และ mobile

## GitHub CI/CD

Workflow แยกเป็น `frontend.yml`, `backend.yml`, `backoffice.yml` แต่ละตัว trigger ตาม path ที่เกี่ยวข้อง, verify, build Docker image และ publish ไป GHCR เมื่อ push main; PR ตรวจ build โดยไม่ publish มี `integration.yml` เปิด Compose และทดสอบ browser ทั้ง stack เพิ่มเติม

Images: `ghcr.io/<owner>/<repo>-frontend`, `-backend`, `-backoffice`; tags `sha-<full commit SHA>` และ `latest` ทั้งสามส่วน build/deploy แยกกันได้ ใช้ `FRONTEND_IMAGE_TAG`, `BACKOFFICE_IMAGE_TAG`, `BACKEND_IMAGE_TAG` แยกกันเมื่อเลือก immutable tags เพราะ commit ที่แก้เพียง app เดียวจะ publish เพียง image ของ app นั้น ระบบนี้รองรับ backend replica เดียวต่อ SQLite volume

ตั้ง repository variables `VITE_GALLERY_URL` และ `VITE_BACKOFFICE_URL` เป็น URL จริงก่อน production build และให้ workflow มี package write permission ผ่าน `GITHUB_TOKEN`

Deploy images ที่ publish แล้วบน host ที่มี Docker:

```sh
export IMAGE_REPOSITORY='owner/repo'
export IMAGE_TAG=latest
# .env มี ADMIN_TOKEN ที่สุ่มใหม่สำหรับ environment นี้
# private GHCR packages ต้อง docker login ghcr.io ก่อน pull
docker compose -f compose.release.yaml pull
docker compose -f compose.release.yaml up -d
```

Release compose bind frontend/back-office ที่ loopback สำหรับ reverse proxy/TLS และไม่เปิด backend ออกภายนอก ขณะนี้มี workflow และ release manifest พร้อม แต่ยังไม่ได้ผูก GitHub remote, publish images หรือ rollout ไป production เพราะยังไม่ได้ระบุ repository/deployment host

สำรอง SQLite ด้วย SQLite backup API (`.backup`) หรือหยุด backend ก่อน copy volume; ไม่ copy เฉพาะ main database ขณะที่ WAL กำลังถูกเขียน
