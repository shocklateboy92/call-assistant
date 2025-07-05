#!/bin/bash

set -e

SYNAPSE_URL="http://synapse:8008"

echo "🚀 Setting up Synapse test users..."

check_synapse_health() {
    echo "Checking Synapse server health..."
    if ! curl -s -f "${SYNAPSE_URL}/_matrix/client/versions" > /dev/null; then
        echo "❌ Synapse server is not responding at ${SYNAPSE_URL}"
        exit 1
    fi
    echo "✅ Synapse server is running"
}

try_login() {
    local username=$1
    local password=$2
    
    local response=$(curl -s -X POST \
        "${SYNAPSE_URL}/_matrix/client/r0/login" \
        -H "Content-Type: application/json" \
        -d "{
            \"type\": \"m.login.password\",
            \"user\": \"${username}\",
            \"password\": \"${password}\"
        }")
    
    if echo "$response" | grep -q '"access_token"'; then
        return 0  # User exists and login successful
    else
        return 1  # User doesn't exist or login failed
    fi
}

create_user() {
    local username=$1
    local password=$2
    
    if try_login "$username" "$password"; then
        echo "✅ User $username already exists"
        return 0
    fi
    
    echo "Creating user: $username"
    
    local response=$(curl -s -X POST \
        "${SYNAPSE_URL}/_matrix/client/r0/register" \
        -H "Content-Type: application/json" \
        -d "{
            \"username\": \"${username}\",
            \"password\": \"${password}\",
            \"auth\": {
                \"type\": \"m.login.dummy\"
            }
        }")
    
    if echo "$response" | grep -q '"access_token"'; then
        echo "✅ Successfully created user: $username"
    else
        echo "❌ Failed to create user: $username"
        echo "Response: $response"
        if echo "$response" | grep -q "M_FORBIDDEN"; then
            echo "⚠️  Registration may be disabled. Check homeserver.yaml for enable_registration: true"
        fi
        exit 1
    fi
}

main() {
    check_synapse_health
    
    echo "Creating test users..."
    create_user "alice" "TestPassword123"
    create_user "bob" "TestPassword123"
    
    echo "🎉 Setup complete!"
}

main "$@"