# Project Context: Destination Cocktails Ecosystem

## 1. Project Overview
**Name:** Destination Cocktails Complete System  
**Client:** Destination Cocktails (Bar/Club)  
**Developer:** Dumu Technologies  
**Goal:** Build a complete ecosystem with three user experiences:
1. **Customer (WhatsApp Bot):** Order drinks from tables via WhatsApp
2. **Bar Staff (WhatsApp Notifications):** Receive and manage order preparation
3. **Manager (PWA Dashboard):** Manage inventory, prices, and view analytics

---

## 2. System Architecture

### Monorepo Structure
```
destination-cocktails/
├── internal/          # Go Backend (Bot Logic + API + SSE)
├── web/              # Next.js PWA Dashboard
├── migrations/       # Database migrations
└── cmd/             # Entry points
```

### Tech Stack

#### Backend (Go)
* **Language:** Go 1.22+
* **Framework:** Fiber v2 (High-performance web framework)
* **Database:** PostgreSQL (Railway)
* **Connection Pooling:** PgBouncer
* **Caching/State:** Redis (Railway) - User sessions
* **ORM:** GORM v2 with `pgx` driver
* **Real-time:** Go Channels + Server-Sent Events (SSE)

#### Frontend (Manager Dashboard)
* **Framework:** Next.js 14 (App Router)
* **UI Library:** ShadCN UI + Tailwind CSS
* **State Management:** TanStack Query (React Query)
* **Real-time Updates:** SSE (EventSource API)
* **Deployment:** PWA (Progressive Web App)

#### Integrations
* **Messaging:** WhatsApp Cloud API (Meta)
* **Payments:** Kopo Kopo (M-Pesa STK Push)
* **Fallback Payments:** Pesapal (Card payments)

---

## 3. User Experiences

### 3.1 Customer Experience (WhatsApp Bot)

#### Menu Browsing
* **Categories:** WhatsApp Interactive Lists (button: "View Menu")
* **Products:** Text Message with numbered list (unlimited items)
* **Selection:** Type number ("1") or name ("Gin")

#### Instant Search (New Feature)
* **Trigger:** Typing any text in START state (e.g., "Jameson")
* **Results:** Numbered list of matches
* **No Results:** Suggests trying again
* **Welcome Message:** "Tap Order Drinks or simply type a drink name to search."

#### Cart & Checkout
* Add to Cart → View Cart → Checkout → Enter Phone for M-Pesa

#### Global Reset
* **Commands:** `hi`, `hello`, `start`, `restart`, `reset`, `menu`
* **Action:** Wipes session (empty cart, state = START), sends welcome message
* **Works:** From any state in the flow

---

### 3.2 Bar Staff Experience (WhatsApp Notifications)

#### Order Notification
* **Trigger:** Every paid order
* **Content:**
  - 4-digit Pickup Code (random)
  - Items list with quantities
  - Customer info
* **Format:** WhatsApp Interactive Button Message

#### "Mark Done" Workflow
* **Button:** [ Mark Done ]
* **Action:** Updates order status to `COMPLETED`
* **Response:** "✅ Order Served"
* **Safety:** Prevents double-clicking (checks if already served)

---

### 3.3 Manager Experience (PWA Dashboard)

#### Access & Security
* **Platform:** Web-based PWA (mobile-optimized)
* **Authentication:** WhatsApp OTP (no passwords)
* **Flow:** Enter phone → Receive code via WhatsApp → Login

#### Live Operations Feed (Home Tab)
* **Real-time:** New orders appear instantly (SSE)
* **Status Indicators:** Paid, Pending, Served (updates live)
* **UI:** Clean list view, no images

#### Inventory Management (Stock Tab)
* **Quick Actions:** +/- buttons to adjust stock
* **Price Editor:** Tap-to-edit prices
* **Optimistic UI:** Changes turn green immediately (fast feel on slow networks)
* **Visuals:** Emoji categories (🍺, 🥃, 🍷) for fast scanning

#### Analytics (Reports Tab)
* **30-Day Summary:** Table/chart showing Date, Total Orders, Total Revenue
* **Daily Snapshot:** Cards showing "Today's Sales" and "Best Seller"

---

## 4. Data Flow & Architecture

### Customer Order Flow
```
1. Customer sends WhatsApp message
2. Backend checks Redis for session (user_session:{phone})
3. Process message based on state (START, MENU, BROWSING, etc.)
4. Update cart in Redis
5. Checkout → Kopo Kopo STK Push
6. Payment webhook → Update order status to PAID
7. Notify bar staff via WhatsApp
8. Notify manager dashboard via SSE
```

### Real-time Synchronization
* **Technology:** Go Channels + SSE (simpler/cheaper than Redis Pub/Sub)
* **Events:**
  - New order created
  - Order status changed (PAID → COMPLETED)
  - Stock level updated
  - Price changed

---

## 5. Database Schema (PostgreSQL)

### `users`
* `id` (UUID, PK)
* `phone_number` (String, Unique, Indexed)
* `name` (String, nullable)
* `created_at` (Timestamp)

### `products`
* `id` (UUID, PK)
* `name` (String)
* `description` (String)
* `price` (Decimal)
* `stock_quantity` (Int)
* `category` (String) - e.g., "Beer", "Whisky", "Chasers"
* `image_url` (String)
* `is_active` (Boolean) - For soft deletes
* `created_at` (Timestamp)
* `updated_at` (Timestamp)

### `orders`
* `id` (UUID, PK)
* `user_id` (FK → users.id)
* `customer_phone` (String)
* `table_number` (String)
* `total_amount` (Decimal)
* `status` (Enum: PENDING, PAID, FAILED, COMPLETED, CANCELLED)
* `payment_method` (Enum: MPESA, CARD, CASH)
* `payment_reference` (String)
* `pickup_code` (String, 4-digit) - For bar staff
* `created_at` (Timestamp)
* `updated_at` (Timestamp)

### `order_items`
* `id` (UUID, PK)
* `order_id` (FK → orders.id)
* `product_id` (FK → products.id)
* `quantity` (Int)
* `price_at_time` (Decimal)
* `created_at` (Timestamp)

### `admin_users` (New)
* `id` (UUID, PK)
* `phone_number` (String, Unique, Indexed)
* `name` (String)
* `role` (Enum: OWNER, MANAGER, STAFF)
* `is_active` (Boolean)
* `created_at` (Timestamp)

### `otp_codes` (New)
* `id` (UUID, PK)
* `phone_number` (String, Indexed)
* `code` (String, 6-digit)
* `expires_at` (Timestamp)
* `verified` (Boolean)
* `created_at` (Timestamp)

---

## 6. API Endpoints

### Customer Bot (Existing)
```
POST   /api/webhooks/whatsapp     - Receive WhatsApp messages
POST   /api/webhooks/payment      - Kopo Kopo payment callback
GET    /api/webhooks/whatsapp     - WhatsApp verification
```

### Manager Dashboard (New)
```
POST   /api/admin/auth/request-otp    - Request WhatsApp OTP
POST   /api/admin/auth/verify-otp     - Verify OTP and login
POST   /api/admin/auth/logout         - Logout
GET    /api/admin/auth/me             - Get current user

GET    /api/admin/products            - List products
PATCH  /api/admin/products/:id/stock  - Update stock
PATCH  /api/admin/products/:id/price  - Update price
PUT    /api/admin/products/:id        - Update product

GET    /api/admin/orders              - List orders (with filters)
GET    /api/admin/orders/:id          - Get order details

GET    /api/admin/analytics/overview  - Dashboard summary
GET    /api/admin/analytics/revenue   - Revenue trends (30 days)
GET    /api/admin/analytics/top-products - Best sellers

GET    /api/admin/events              - SSE stream for real-time updates
```

### Bar Staff (New)
```
POST   /api/bar/orders/:id/complete   - Mark order as completed
```

---

## 7. Key Features & Changes

### New Features
1. **Instant Search:** Type drink name in START state
2. **Global Reset:** Commands work from any state
3. **Bar Staff Notifications:** WhatsApp messages with "Mark Done" button
4. **Pickup Codes:** 4-digit codes for order identification
5. **Manager Dashboard:** PWA with WhatsApp OTP login
6. **Real-time Updates:** SSE for live order feed
7. **Optimistic UI:** Instant feedback on stock/price changes

### Data Changes
* **Menu Update:** Replaced "Cognac" with "Chasers" (Coke, Ice, Red Bull, etc.)
* **Soft Deletes:** Old items archived (`is_active = false`) to preserve order history
* **Product IDs:** UUIDs used internally; simple numbers used in customer chat
* **Order Status:** Added `COMPLETED` status (when bar staff marks done)
* **Pickup Codes:** Added to orders table

---

## 8. Coding Standards & Rules

### Go Backend
* **No Magic Strings:** Use constants for Redis keys, order statuses
* **Error Handling:** Explicit error handling, never ignore errors
* **Environment Variables:** All secrets in `.env`
* **Comments:** Document complex logic (payment webhooks, SSE)
* **Redis TTL:** All sessions have TTL (2 hours)

### Next.js Frontend
* **TypeScript:** Strict mode enabled
* **Components:** Reusable ShadCN components
* **API Calls:** Use TanStack Query for caching
* **Optimistic Updates:** Immediate UI feedback
* **Error Handling:** Toast notifications for errors

---

## 9. Deployment (Railway)

### Single Project, Two Services
```
Service 1: Go Backend (Port 8080)
  - /api/webhooks/*     → WhatsApp bot
  - /api/admin/*        → Dashboard API
  - /api/bar/*          → Bar staff API
  - /api/admin/events   → SSE stream

Service 2: Next.js Frontend (Port 3000)
  - PWA Dashboard
```

### Shared Resources
* **PostgreSQL:** Single database for all data
* **Redis:** Single instance for sessions + real-time events

---

## 10. Security Considerations

* **WhatsApp Webhook:** Verify signature on all incoming messages
* **Payment Webhook:** Verify Kopo Kopo signature
* **Admin Auth:** WhatsApp OTP (6-digit, 5-minute expiry)
* **API Protection:** JWT tokens in HTTP-only cookies
* **Rate Limiting:** Prevent OTP spam
* **CORS:** Restrict to dashboard domain only