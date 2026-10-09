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

Gallery ใช้ธีม dark blueและตัวกรองขนาด 12–16px อ่านข้อมูลจาก API และรองรับค้นหาชื่อ/กลุ่ม/source, filter กลุ่มและช่วงวันที่, เรียงล่าสุด/เก่าสุด/ชื่อ, group ตามศิลปินหรือวันที่, lazy load ทีละ 24 ชุดเมื่อเลื่อนใกล้ท้ายรายการ (ไม่มีปุ่มเปลี่ยนหน้า), carousel บนแต่ละ card (แถบกึ่งกลางด้านล่างรูป กดค้างแล้วลากซ้าย/ขวาด้วยเมาส์หรือ touch; รองรับ ArrowLeft/ArrowRight และ Home/End เมื่อ focus ที่แถบ) และ dialog รายละเอียดพร้อมรูปและลิงก์ example post Filter อยู่ใน URL เพื่อแชร์มุมมองได้

ปุ่มมุมมองข้างชื่อ collection สลับการ์ดกับตารางคล้าย Google Sheet ตารางแสดง SET_ID, วันที่, รูปย่อ/ชื่อชุด, ศิลปิน, shop/source, จำนวนรูป, example และ notes; เลื่อนแนวนอนเพื่อดูคอลัมน์ทั้งหมด กดชื่อชุดเพื่อเปิดรายละเอียด ทั้งสองมุมมองใช้ filter/group/sort และ lazy load เดียวกัน โดยไม่ล้างชุดที่โหลดแล้วเมื่อสลับ มุมมองตารางอยู่ใน URL (`view=table`)

ครั้งแรกมี short guide ภาษาอังกฤษ 3 ขั้นพร้อมตัวอย่างเคลื่อนไหว: มุมมอง, filter, การเปิด/เลื่อนรูปและโหลดเพิ่ม ติ๊ก **Don't show it again** แล้วปิดเพื่อจำค่าใน browser นี้ เปิดดูซ้ำได้จากปุ่ม **?** ข้างปุ่มมุมมอง และเอาติ๊กออกเพื่อให้แสดงเมื่อเข้าครั้งถัดไป รองรับ Escape และลด animation ตาม reduced-motion ของระบบ

ส่วน Contact บนหน้าแรกแสดง [Ganknow @OtakuICETEA](https://ganknow.com/OtakuICETEA) และ [Telegram @madaomg](https://t.me/madaomg) โดยเปิดลิงก์ในแท็บใหม่

ป้าย group บนการ์ดใช้ขนาด 12px (11px บนจอแคบ) เท่าป้าย shop พร้อมพื้นหลังน้ำเงิน ตัวอักษรสว่าง และขึ้นบรรทัดใหม่เมื่อชื่อยาว ป้าย source/shop บนการ์ดและหน้ารายละเอียดขยายขนาดพร้อมสีพื้นหลังประจำร้าน (ชื่อร้านเดิมใช้สีเดิม รวม alias `datacoffeeshop (?)`) การ์ดบน desktop ลอยขึ้น เอียงตามเมาส์ และมีแสง sheen โดยคืนตำแหน่งเมื่อออกจากการ์ด การลาก carousel จะพักการเอียงทันที; touchscreen และ reduced motion แสดงการ์ดนิ่ง

Back-office เพิ่มและแก้ไข set ด้วยมือ, แสดง `set_id` ในตารางและ editor, อัปโหลด JPEG/PNG/GIF/WebP ให้แต่ละชุดมี 2–5 รูปที่ไม่ซ้ำกัน (ไม่เกิน 12 MiB/รูป), import Google Sheet ที่อ่านได้โดยไม่ต้อง sign in หรือ CSV export และสั่ง extract/retry preview ที่ยังขาดได้ รูปจะตรวจชนิดและจำนวน pixel แล้วปรับขนาดไม่เกิน 1200 pixels ต่อด้านก่อนเก็บเป็น JPEG ใน SQLite

ชุดที่นำเข้าหรือชุดเดิมซึ่งยังมี 0–1 รูปจะคง metadata ไว้และแสดง `needs 2+` จนเติมรูปได้ครบ API จำกัดสูงสุด 5 รูปต่อชุดทั้งจาก upload และ extraction การอัปโหลดที่ทำให้เกิน 5 หรือยังมีไม่ถึง 2 รูปจะยกเลิกทั้ง batch โดยตรวจจาก hash ของรูปจริง

CSV template ดาวน์โหลดได้จากหน้า Import & previews หรือไฟล์ `backoffice/public/template.csv`:

```csv
ID,Date,Name,GROUP,Source,Example,REMARK
set-001,260915,260915 Karina,aespa,kdatastudio,https://ganknow.com/post/5ddbab87-f0f2-404a-89a9-eb2f897aad61,Example
```

`ID` ไม่บังคับ แต่ช่วยให้เปลี่ยนชื่อ/กลุ่ม/วันที่แล้วอัปเดต record เดิมได้ ถ้าไม่มี ID ใช้ source sheet + กลุ่ม + วันที่ดิบ + ชื่อเป็น identity พร้อม occurrence สำหรับแถวซ้ำ วันที่รองรับ YYMMDD หรือ YYYY-MM-DD; blank/0 เป็น unknown และวันที่ผิดจะมี warning ทุกคอลัมน์ต้นฉบับถูกเก็บไว้

กด **Scan Google Sheet & import** จากหน้าหลัก back-office หรือเลือก CSV ใน Import & previews เพื่อแสดง modal ผลสแกนก่อนบันทึก: ชุดใหม่, metadata ที่เปลี่ยนพร้อมค่าก่อน/หลัง, จำนวนแถวเดิม และจำนวนชุดที่จะดึงรูป กด Cancel จะไม่เปลี่ยนข้อมูล กด **OK — Update** จะนำเข้า CSV snapshot ที่ตรวจไว้ แล้วดึงสูงสุด 5 รูปเฉพาะชุดใหม่หรือ Example ที่เปลี่ยน ไม่ต้องกด Extract เพิ่ม เปิดแท็บนี้ไว้ระหว่างดึง; ถ้า Stop หรือพบ cooldown จะมีปุ่ม Continue imported previews สำหรับรายการที่เหลือในแท็บนี้

Import ซ้ำไม่เพิ่ม record ซ้ำใน identity เดิม อัปเดต metadata ของแถวที่ตรงกันและคง manual records ไว้ ไม่ลบ record ที่หายไปจาก Sheet เปลี่ยน Example แล้วล้าง preview เดิมเพื่อไม่แสดงรูปผิดชุด การแก้ record ที่มาจาก Sheet ด้วยมืออาจถูก import ครั้งถัดไปเขียนทับ; ใช้ Sheet เป็นต้นฉบับสำหรับ record นั้น

## ข้อมูลเริ่มต้น

ใช้ข้อมูลทั้งหมดจาก [Google Sheet ที่ให้มา](https://docs.google.com/spreadsheets/d/1z45mKRtLvTZth8b-uhqkMyjzxVtHI8b7YVcySQVeEkg/edit?gid=0) export เมื่อ 6 ตุลาคม 2026 โดยไม่ใช้ filtered view ที่ติดมากับ URL:

- `data/source.csv`: snapshot ต้นฉบับ 165 data rows, 31 กลุ่ม หัวตารางจริงอยู่แถวที่ 4
- `data/seed.sqlite`: metadata ทุกแถวและ preview จริงที่ดาวน์โหลดได้ ไม่ต้องต่อ Google ตอนเปิดระบบครั้งแรก
- seed ล่าสุด: 550 preview records, 116 ชุดมีครบ 2–5 รูป (4 ชุดมี 2 รูป, 6 ชุดมี 3 รูป, 7 ชุดมี 4 รูป, 99 ชุดมี 5 รูป), 1 ชุดมีเพียง 1 รูป และ 48 ชุดยังไม่มีรูป โดย 37 แถวไม่มี public example URL; รอบล่าสุดเติมกลุ่มที่เดิมมี 2 รูปครบทั้ง 113 ชุด เพิ่ม 310 รูป และ 109 ชุดเพิ่มเป็น 3–5 รูป ดู `docs/IMAGE_FETCH_PROGRESS.log` และ `docs/IMAGE_FETCH_TWO_TO_FIVE_2026_10_07.json`
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

เมื่อผู้ดูแลต้องการเพิ่มงบชั่วคราว ใช้ CLI เพิ่มครั้งละ 1–60 คำขอโดยเก็บประวัติเดิมไว้ เช่น stack แบบ development:

```sh
docker compose exec -T backend go run ./cmd/server -db /data/gallery.sqlite -grant-gank-requests=60
```

งบเพิ่มเก็บใน SQLite และหมดอายุเมื่อคำขอแรกในช่วง 24 ชั่วโมงปัจจุบันเริ่มพ้นช่วงนั้น จากนั้นกลับไปใช้งบปกติ 60 คำขอ การเพิ่มงบล้างได้เฉพาะช่วงพักที่เกิดจากงบของแอปเอง; หากต้นทางตอบ 429/403 หรือ challenge และยังอยู่ในช่วงพัก CLI จะปฏิเสธการเพิ่มงบ ระยะห่าง 20 วินาทีและ lease ยังคงเดิม `GET /api/admin/ingestion` แสดงงบรวมและคำขอที่เหลือจริง

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
.github/workflows/        CI/CD production พร้อม browser integration
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
| `POST /api/admin/import/preview` | JSON `sheetUrl` หรือ multipart CSV; dry-run report + exact CSV snapshot, ไม่บันทึกข้อมูล |
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

`cicd.yml` รวม verify, browser acceptance, build/publish และ production deploy ใน workflow เดียว อิงรูปแบบ [workflow ของ gank-data-finder](https://github.com/otakuict/gank-data-finder/blob/main/.github/workflows/cicd.yml) ทุก commit ที่ push `main` จะสร้างทั้งสาม image เพื่อให้ release ใช้ SHA เดียวกันทั้งหมด:

- `<DOCKERHUB_USERNAME>/bias-archive-frontend`
- `<DOCKERHUB_USERNAME>/bias-archive-backoffice`
- `<DOCKERHUB_USERNAME>/bias-archive-backend`

PR เข้า `main` และ manual run จาก branch อื่นตรวจ Node/Go, browser และ Docker build โดยไม่ login/publish/deploy และมี namespace `ci-local` สำหรับ PR ที่ไม่มี repository variable เมื่อเป็น `main` จะ push tags แบบ full commit SHA และ `latest` แล้ว deploy ด้วย SHA เท่านั้น Deploy ต้องรอ browser tests และ image ทั้งสามสำเร็จ workflow concurrency ป้องกัน release บน `main` ทำงานทับกัน และยกเลิกเฉพาะ PR ที่กำลังรันเมื่อมี commit ใหม่ งาน main ที่ยังรอคิวอาจถูกแทนที่ด้วย commit ใหม่กว่าได้

Dockerfile ทั้งสามใช้ multi-stage: Node → nginx สำหรับสองเว็บ และ Go → Debian slim สำหรับ API Runtime image ไม่รวม compiler/Node build dependencies; healthcheck ของเว็บตรวจ API ผ่าน nginx และ backend ตรวจ database readiness `ADMIN_TOKEN` รับตอนเริ่ม container เท่านั้น ส่วน `VITE_*` เป็น URL สาธารณะซึ่งฝังใน JavaScript ตอน build

### ตั้งค่า GitHub ก่อนใช้งาน

Repository variables:

- `DOCKERHUB_USERNAME` — บัญชีเจ้าของทั้งสาม Docker Hub repositories
- `VITE_GALLERY_URL`, `VITE_BACKOFFICE_URL` — URL HTTPS สาธารณะของสองเว็บ ต้องตั้งก่อน publish บน `main`; PR ใช้ localhost ได้หากไม่มี variable

Repository secret:

- `DOCKERHUB_TOKEN` — Docker Hub access token ที่ push/pull images ได้ ใช้โดย image job

สร้าง GitHub Environment ชื่อ `production`, จำกัด deployment branch เป็น `main` และตั้ง:

- Secret `ADMIN_TOKEN` — token สุ่มอย่างน้อย 24 ตัวอักษร ใช้เฉพาะ deploy/runtime
- Secret `DOCKERHUB_TOKEN` — หากต้องการ credential สำหรับ deploy แยกจาก build ให้ใช้ token ที่ pull ได้ในบัญชีเดียวกัน; หากไม่ตั้งจะใช้ repository secret
- Optional variables `GALLERY_PORT`, `BACKOFFICE_PORT` — default 5173/5174
- Optional variable `DEPLOY_PROJECT_NAME` — default `otakuict-data-gallery` ซึ่งตรงกับชื่อ project เดิมของ release compose ใน checkout นี้ หากระบบเดิมใช้ `-p` หรือ `COMPOSE_PROJECT_NAME` ให้ตั้งชื่อนั้นเพื่อ reuse volume เดิม

อย่า override `DOCKERHUB_USERNAME` เป็นบัญชีอื่นใน Environment เพราะ image job ใช้ repository variable งานที่อ้าง Environment จะเข้าถึง secrets หลังผ่าน protection rules ตาม [GitHub documentation](https://docs.github.com/en/actions/how-tos/deploy/configure-and-manage-deployments/manage-environments)

ติดตั้ง self-hosted runner บน production host Linux x64 พร้อม Docker Engine และ Compose v2 ที่รองรับ `up --wait` ใส่ labels `self-hosted`, `linux`, `x64`, `production` และสิทธิ์เข้าถึง Docker socket เฉพาะ deploy job เท่านั้นที่ใช้ runner นี้ ส่วน PR/verify/build ใช้ GitHub-hosted runner

Deploy job login โดยใช้ Docker config directory แยกใน runner temp แล้วเรียก `bash scripts/deploy-server.sh "$SHA"` Script ตรวจ secret และ full SHA, pull ทั้งชุดก่อนเปลี่ยน container, เริ่ม backend ก่อนเว็บตาม health dependency แล้วรอ healthchecks ภายใน 180 วินาที ไม่อ่าน `.env` ใน checkout และไม่ลบ volume SQLite หาก pull ล้มเหลวจะไม่เริ่ม rollout; หาก healthcheck ล้มเหลว job จะ fail โดยไม่มี automatic rollback

### Deploy หรือ rollback ด้วยมือ

บน host เดิมที่ login Docker Hub แล้ว ให้ export runtime configuration จาก secret store ของ host (ไม่ใส่ token จริงใน command history):

```sh
export DOCKERHUB_USERNAME='your-dockerhub-username'
# Export ADMIN_TOKEN from your host secret store; keep the same token on a rollback.
# Set DEPLOY_PROJECT_NAME to the existing Compose project if it differs from the default.
bash scripts/deploy-server.sh '<full commit SHA whose three images were published>'
```

ใช้ SHA ของ release ก่อนหน้าเพื่อ rollback image ทั้งชุด การ rollback ไม่ rollback schema/data และ script ไม่ลบ volume ข้อมูล ระบบรองรับ backend replica เดียวต่อ SQLite volume Release compose bind สองเว็บที่ loopback สำหรับ reverse proxy/TLS และไม่เปิด backend port ภายนอก ต้องตั้ง reverse proxy/TLS บน host แยกต่างหาก

สำรอง SQLite ด้วย SQLite backup API (`.backup`) หรือหยุด backend ก่อน copy volume; ไม่ copy เฉพาะ main database ขณะที่ WAL กำลังถูกเขียน
