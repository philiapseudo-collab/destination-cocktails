# Project Context: Destination Cocktails (Dumu Technologies)

## 1. Project Overview
**Name:** Destination Cocktails Bot
**Client:** Destination Cocktails (Bar/Club)
**Developer:** Dumu Technologies
**Goal:** Build a high-concurrency WhatsApp chatbot that allows bar patrons to order drinks/food directly from their tables. The system must handle hundreds of concurrent requests (Friday night traffic) without lagging. It includes a Real-Time Admin Dashboard for stock management and order tracking.

## 2. Tech Stack (Strict)
We are prioritizing **throughput**, **concurrency**, and **type safety**.

### Backend (The Core)
* **Language:** Go (Golang) 1.22+
* **Framework:** Fiber v2 (Go web framework optimized for speed).
* **Database:** PostgreSQL (Hosted on Railway).
* **Connection Pooling:** PgBouncer (Must be configured to handle high connection churn).
* **Caching/State:** Redis (Used for user session state, e.g., `current_menu_level`, `cart_items`).
* **ORM/SQL:** GORM (v2) - *Strictly used with the `pgx` driver for performance.*

### Frontend (Admin Dashboard)
* **Framework:** Next.js 14 (App Router).
* **UI Library:** ShadCN UI + Tailwind CSS.
* **State Management:** TanStack Query (React Query) for real-time data fetching.

### Integrations
* **Messaging:** WhatsApp Cloud API (Meta).
* **Payments (Primary):** Kopo Kopo (K2 Connect) - M-Pesa STK Push.
* **Payments (Fallback/Card):** Pesapal - Used only if M-Pesa fails or user requests Card.

## 3. Architecture & Data Flow
1.  **User Interaction:** User sends a message on WhatsApp.
2.  **State Check:** Backend checks Redis for `user_session:{phone_number}`.
    * If key exists: Retrieve current state (e.g., `BROWSING_BEERS`).
    * If key missing: Create new session.
3.  **Processing:**
    * **Menu Navigation:** Handled via static logic + Redis. No DB hits.
    * **Ordering:** Updates `cart` in Redis.
    * **Checkout:** Triggers Kopo Kopo STK Push.
4.  **Payment Webhook:**
    * Kopo Kopo sends webhook to `/api/webhooks/payment`.
    * **Signature Verification:** Backend MUST verify `X-KopoKopo-Signature`.
    * **Success:** Write Order to PostgreSQL -> Clear Redis Cart -> Notify Admin Dashboard via WebSocket/SSE.
    * **Failure:** Send fallback message with Pesapal link.

## 4. Database Schema (PostgreSQL)

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
* `stock_quantity` (Int) - *Critical for inventory management*
* `category` (String) - e.g., "Beer", "Whisky", "Cocktail"
* `image_url` (String)
* `is_active` (Boolean)

### `orders`
* `id` (UUID, PK)
* `user_id` (FK -> users.id)
* `total_amount` (Decimal)
* `status` (Enum: PENDING, PAID, FAILED, SERVED, CANCELLED)
* `payment_method` (Enum: MPESA, CARD, CASH)
* `payment_reference` (String) - e.g., M-Pesa Code
* `table_number` (String)
* `created_at` (Timestamp)

### `order_items`
* `id` (UUID, PK)
* `order_id` (FK -> orders.id)
* `product_id` (FK -> products.id)
* `quantity` (Int)
* `price_at_time` (Decimal)

## 5. Coding Standards & Rules
* **No "Magic" Strings:** Use constants for all Redis keys and Order Statuses.
* **Error Handling:** Go error handling must be explicit. Do not ignore errors.
* **Environment Variables:** All secrets (API Keys, DB URLs) must be loaded from `.env`.
* **Comments:** Comment complex logic, especially the payment webhook verification.
* **Redis TTL:** All user sessions in Redis must have a TTL (e.g., 2 hours) to prevent memory leaks.