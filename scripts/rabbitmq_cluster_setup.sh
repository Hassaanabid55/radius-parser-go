#!/bin/bash
set -e

# =========================================================
# DEFAULT CONFIG (can be overridden via env)
# =========================================================

while [[ $# -gt 0 ]]; do
  case $1 in
    --master) MASTER="$2"; shift 2 ;;
    --nodes) NODES="$2"; shift 2 ;;
    --user) USER="$2"; shift 2 ;;
    --sshpass) SSHPASS="$2"; shift 2 ;;
    --rabbit-user) RABBIT_USER="$2"; shift 2 ;;
    --rabbit-pass) RABBIT_PASS="$2"; shift 2 ;;
    --vhost) VHOST="$2"; shift 2 ;;
    --exchange) EXCHANGE="$2"; shift 2 ;;
    --rabbit-node) RABBIT_NODE="$2"; shift 2 ;;
    *)
      echo "Unknown parameter: $1"
      exit 1
      ;;
  esac
done

COOKIE_PATH="/var/lib/rabbitmq/.erlang.cookie"

# normalize + convert string → array safely
NODES="${NODES:-}"
NODES=$(echo "$NODES" | tr '\n' ' ' | tr -s ' ')
IFS=' ' read -r -a RABBIT_NODES <<< "$NODES"

# =========================================================
# SSH HELPERS
# =========================================================

if [ -z "$SSHPASS" ]; then
  SSH="ssh -o StrictHostKeyChecking=no"
  SCP="scp -o StrictHostKeyChecking=no"
else
  SSH="sshpass -p $SSHPASS ssh -o StrictHostKeyChecking=no"
  SCP="sshpass -p $SSHPASS scp -o StrictHostKeyChecking=no"
fi

echo "========================================"
echo " RabbitMQ Cluster Bootstrap"
echo "========================================"
echo "Master     : $MASTER"
echo "Nodes      : $NODES"
echo "User       : $USER"
echo "RabbitUser : $RABBIT_USER"
echo "VHost      : $VHOST"
echo "Exchange   : $EXCHANGE"
echo "RabbitNode : $RABBIT_NODE"
echo "========================================"

# =========================================================
# 1. SYNC ERLANG COOKIE
# =========================================================

echo "[1/5] Syncing Erlang cookie..."

$SCP ${USER}@${MASTER}:${COOKIE_PATH} ./cookie

for n in "${RABBIT_NODES[@]}"; do
    echo "Copying cookie -> $n"
    echo "rm -rf /tmp/.erlang.cookie on $n"

    $SSH ${USER}@${n} "
        rm -rf /tmp/.erlang.cookie
    "

    $SCP ./cookie ${USER}@${n}:/tmp/.erlang.cookie

    $SSH ${USER}@${n} "
        mv /tmp/.erlang.cookie /var/lib/rabbitmq/.erlang.cookie &&
        chown rabbitmq:rabbitmq /var/lib/rabbitmq/.erlang.cookie &&
        chmod 400 /var/lib/rabbitmq/.erlang.cookie
    "
done

rm -f ./cookie

# =========================================================
# 2. START MASTER
# =========================================================

echo "[2/5] Starting master..."

$SSH ${USER}@${MASTER} "
rabbitmqctl stop_app || true
rabbitmqctl start_app
"

# =========================================================
# 3. FORM CLUSTER
# =========================================================

join_node () {
    local node=$1

    echo "Joining $node..."

    $SSH ${USER}@${node} "
        rabbitmqctl stop_app || true
        rabbitmqctl reset || true
        rabbitmqctl join_cluster ${RABBIT_NODE} || true
        rabbitmqctl start_app
    "
}

echo "[3/5] Forming cluster..."

CLUSTER_NODES=("${RABBIT_NODES[@]}")
MASTER_NODE_INDEX="$MASTER"

for n in "${CLUSTER_NODES[@]}"; do
    [[ "$n" == "$MASTER" ]] && continue
    join_node "$n"
done

# =========================================================
# 4. VERIFY CLUSTER
# =========================================================

echo "[4/5] Cluster status..."

$SSH ${USER}@${MASTER} "rabbitmqctl cluster_status"

# =========================================================
# 5. PROVISION RADIUS (ON MASTER)
# =========================================================

echo "[5/5] Provisioning RabbitMQ resources..."

$SSH ${USER}@${MASTER} "
set -e

RABBITMQ_USER='${RABBIT_USER}'
RABBITMQ_PASS='${RABBIT_PASS}'
VHOST='${VHOST}'
EXCHANGE='${EXCHANGE}'

echo 'Creating vhost...'
rabbitmqctl add_vhost \$VHOST 2>/dev/null || true

echo 'Creating user...'
rabbitmqctl add_user \$RABBITMQ_USER \$RABBITMQ_PASS 2>/dev/null || true

rabbitmqctl set_user_tags \$RABBITMQ_USER administrator
rabbitmqctl set_permissions -p \$VHOST \$RABBITMQ_USER '.*' '.*' '.*'

echo 'Creating exchange...'
rabbitmqadmin --vhost=\$VHOST \
  --username=\$RABBITMQ_USER \
  --password=\$RABBITMQ_PASS \
  declare exchange \
  --name=\$EXCHANGE \
  --type=topic \
  --durable=true

create_queue () {
    rabbitmqadmin --vhost=\$VHOST \
      --username=\$RABBITMQ_USER \
      --password=\$RABBITMQ_PASS \
      declare queue \
      --name=\$1 \
      --durable=true \
      --arguments '{\"x-queue-type\":\"quorum\"}'
}

bind_queue () {
    rabbitmqadmin --vhost=\$VHOST \
      --username=\$RABBITMQ_USER \
      --password=\$RABBITMQ_PASS \
      declare binding \
      --source=\$EXCHANGE \
      --destination-type=queue \
      --destination=\$1 \
      --routing-key=\$1
}

echo 'Creating queues...'
for q in session.start session.stop session.stats session.final bootstrap.cgnat bootstrap.whitelist; do
    create_queue \$q
done

echo 'Creating bindings...'
for q in session.start session.stop session.stats session.final bootstrap.cgnat bootstrap.whitelist; do
    bind_queue \$q
done

echo 'DONE'
"

# =========================================================
# DONE
# =========================================================

echo "========================================"
echo " RabbitMQ Cluster + Radius READY"
echo "========================================"