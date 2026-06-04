#!/bin/bash
# =============================================================
# Radius Parser RabbitMQ Setup
# Topic Exchange + Quorum Queues
# =============================================================

set -e

RABBITMQ_USER="radius_user"
RABBITMQ_PASS="radius_pass"

VHOST="radius"
EXCHANGE="radius_exchange"

echo "======================================="
echo " Radius Parser RabbitMQ Setup"
echo "======================================="

# =============================================================
# 0. CLEAN OLD CONFIG
# =============================================================

echo "[0/7] Cleaning old vhost..."

rabbitmqctl delete_vhost "$VHOST" 2>/dev/null || true

# =============================================================
# 1. CREATE VHOST
# =============================================================

echo "[1/7] Creating vhost..."

rabbitmqctl add_vhost "$VHOST"

# =============================================================
# 2. CREATE USER
# =============================================================

echo "[2/7] Creating user..."

rabbitmqctl add_user "$RABBITMQ_USER" "$RABBITMQ_PASS" 2>/dev/null || true

rabbitmqctl set_user_tags \
    "$RABBITMQ_USER" administrator

rabbitmqctl set_permissions \
    -p "$VHOST" \
    "$RABBITMQ_USER" \
    ".*" ".*" ".*"

# =============================================================
# 3. CREATE TOPIC EXCHANGE
# =============================================================

echo "[3/7] Creating topic exchange..."

rabbitmqadmin \
  --vhost="$VHOST" \
  --username="$RABBITMQ_USER" \
  --password="$RABBITMQ_PASS" \
  declare exchange \
  --name="$EXCHANGE" \
  --type=topic \
  --durable=true

# =============================================================
# 4. QUORUM QUEUE HELPER
# =============================================================

declare_quorum_queue() {

    QUEUE_NAME=$1

    rabbitmqadmin \
      --vhost="$VHOST" \
      --username="$RABBITMQ_USER" \
      --password="$RABBITMQ_PASS" \
      declare queue \
      --name="$QUEUE_NAME" \
      --durable=true \
      --arguments='{"x-queue-type":"quorum"}'
}

echo "[4/7] Creating queues..."

#
# FILTER APPLICATIONS
#

declare_quorum_queue session.start
declare_quorum_queue session.stop

#
# PARSER CONSUMES STATS
#

declare_quorum_queue session.stats

#
# PARSER CONSUMES BOOTSTRAP DATA
#

declare_quorum_queue bootstrap.cgnat
declare_quorum_queue bootstrap.whitelist

#
# TASK MANAGER CONSUMES FINAL SESSIONS
#

declare_quorum_queue session.final

# =============================================================
# 5. BINDING HELPER
# =============================================================

bind_queue() {

    KEY=$1

    rabbitmqadmin \
      --vhost="$VHOST" \
      --username="$RABBITMQ_USER" \
      --password="$RABBITMQ_PASS" \
      declare binding \
      --source="$EXCHANGE" \
      --destination-type=queue \
      --destination="$KEY" \
      --routing-key="$KEY"
}

echo "[5/7] Creating bindings..."

#
# Parser -> Filters
#

bind_queue session.start
bind_queue session.stop

#
# Filters -> Parser
#

bind_queue session.stats

#
# Task Manager -> Parser
#

bind_queue bootstrap.cgnat
bind_queue bootstrap.whitelist

#
# Parser -> Task Manager
#

bind_queue session.final

# =============================================================
# 6. VERIFY
# =============================================================

echo "[6/7] Verifying..."

echo ""
echo "Queues"
rabbitmqctl list_queues \
    -p "$VHOST" \
    name type durable messages consumers

echo ""
echo "Bindings"
rabbitmqctl list_bindings \
    -p "$VHOST"

# =============================================================
# 7. SUMMARY
# =============================================================

echo ""
echo "======================================="
echo " Setup Complete"
echo "======================================="
echo ""
echo "Exchange:"
echo "  $EXCHANGE"
echo ""
echo "Routing Keys:"
echo "  session.start"
echo "  session.stop"
echo "  session.stats"
echo "  session.final"
echo "  bootstrap.cgnat"
echo "  bootstrap.whitelist"
echo ""
echo "Queues:"
echo "  session.start"
echo "  session.stop"
echo "  session.stats"
echo "  session.final"
echo "  bootstrap.cgnat"
echo "  bootstrap.whitelist"
echo ""
echo "RabbitMQ ready."