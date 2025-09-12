# Telegram AI Subscription Platform

This project is a sophisticated, production-grade Telegram bot that provides users with subscription-based access to various AI language models (like GPT and Gemini). It features a complete payment and subscription management system, a fully localized Persian interface, and a robust set of administrative tools.

The application is built on a high-performance, scalable backend using a modern Go technology stack and follows industry-best practices for architecture and testing.

## Core Features (User-Facing)

* **Mandatory Onboarding**: A guided, conversational registration flow for new users, requiring them to provide a full name and share their phone contact before they can access the bot.
* **Fully Button-Driven UI**: All user interactions are handled through a professional, seamless flow of inline and reply keyboards. A persistent menu provides easy access to core features.
* **Full Persian Localization**: The entire bot interface, including all messages, buttons, and menus, is localized in Persian and managed from a central configuration file.
* **Multi-Tier Subscription Plans**: Users can view and purchase different subscription plans, each with its own price, duration, credit allotment, and list of supported AI models.
* **Dual Purchase Options**:
    * **Payment Gateway**: A fully integrated payment flow using the ZarinPal payment gateway.
    * **Activation Codes**: Users can redeem pre-generated activation codes to subscribe to a plan.
* **Interactive AI Chat**: Users can start chat sessions with any AI model supported by their active plan. The chat system is asynchronous, providing instant feedback while AI responses are generated in the background.
* **User Settings**: A `/settings` command that allows users to manage their privacy preferences, such as enabling or disabling the storage of their chat message history.

## Core Features (Admin-Facing)

* **Dynamic Admin Menu**: Administrators see an enhanced, persistent menu in Telegram with additional buttons for management commands.
* **Plan Management**: A full suite of commands for managing subscription plans:
    * `/create_plan <Name> <Days> <Credits> <Price> <Models-comma-separeted>`: Creates a new subscription plan with a comma-separated list of supported models.
    * `/update_plan <ID> <Name> <Days> <Credits> <Price>`: Updates the details of an existing plan.
    * `/delete_plan <ID>`: Deletes a plan, with safeguards to prevent deleting a plan that is currently in use.
* **Pricing Management**:
    * `/update_pricing <ModelName> <InputPrice> <OutputPrice>`: Updates the per-token credit cost for any AI model.
* **Activation Code Generation**:
    * `/generate_code <PlanID> [Count]`: Generates a specified number of secure, single-use activation codes for a given plan, which are displayed in a copyable format.

## Technical Architecture

* **Hexagonal Architecture (Ports & Adapters)**: The codebase is built on a clean, maintainable architecture that separates core business logic (`domain`, `usecase`) from infrastructure concerns (`infra`). This makes the system highly testable and easy to extend.
* **Asynchronous Processing**: The AI chat functionality uses the **Outbox Pattern**. User messages are queued as jobs in the database, and background workers process them asynchronously. This ensures the bot is always responsive, even under heavy load or when AI models are slow.
* **Technology Stack**:
    * **Language**: Go (Golang)
    * **Database**: PostgreSQL
    * **Cache & State Management**: Redis
    * **Observability**: Prometheus for metrics, Loki for logging, and Grafana for dashboards.
* **Testing**: The project has a comprehensive test suite, including:
    * **Unit Tests** for all use cases and business logic.
    * **Integration Tests** for all database repositories, running against a real, containerized PostgreSQL instance.

## Deployment Guide

This application is designed for a production-like deployment using Docker and Docker Compose.

### Prerequisites
* Docker and Docker Compose installed.
* Git installed.
* A pre-configured `macvlan` network for local development (optional, see `setup_lan_net.sh`).

### 1. Configuration

1.  **Clone the Repository**:
    ```bash
    git clone https://github.com/moligarch/ai-subscription-platform.git
    cd https://github.com/moligarch/ai-subscription-platform.git
    ```
2.  **Create an Environment File**: Copy the example environment/config file to create your local configuration.
    ```bash
    cp .env.example .env
    cp config.yaml.example config.yaml
    ```
3.  **Edit `.env`**: Open the `.env` file and fill in all the required secrets:
    * `BOT_TOKEN`: Your Telegram Bot API token.
    * `DATABASE_URL`: The full connection string for your PostgreSQL database.
    * `REDIS_URL`: The address for your Redis instance.
    * `SECURITY_ENCRYPTION_KEY`: A 32-character (256-bit) key for encrypting sensitive data.
    * `AI_..._API_KEY`: Your API keys for OpenAI and Gemini.
    * `PAYMENT_...`: Your ZarinPal merchant ID.
4.  **Edit `config.yaml`**: Open `config.yaml` and configure your list of `admin_ids` and any other non-secret parameters.

### 2. Running the Application

* **Start All Services**: To start the application and all its dependencies (Postgres, Redis, Prometheus, etc.), run:
    ```bash
    make docker-up
    ```
* **Stop All Services**: To stop all containers and remove their associated volumes, run:
    ```bash
    make docker-down
    ```
* **Local Development**: To run the database and Redis in Docker while you run the Go application locally in your IDE for debugging:
    1.  Start the dependencies: `make docker-run service=postgres service=redis`
    2.  (If using `macvlan`) Configure your host network: `sudo ./setup_lan_net.sh up`
    3.  Run the application from your IDE's debugger.

## Future Work (Roadmap)

The project has a clear roadmap for future development, including:

* **Phase 1: Admin Capabilities & Management UI**
    * Implement an admin `/cast` command for broadcasting messages to all users.
    * Build a secure backend API for a graphical management panel.
    * Develop a modern Single-Page Application (SPA) for the management UI with a dashboard, user/plan management, and a log viewer.
* **Phase 2: Final Polish & Documentation**
    * Perform deep architectural refactoring (Event Bus, Wire DI) to further enhance scalability and maintainability.
    * Add comprehensive `go doc` comments to all public functions and types.
* **Phase 3: Performance & Stability Validation**
    * Write Go benchmarks for performance-critical functions.
    * Conduct load testing to ensure stability under high user traffic.