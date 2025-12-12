#!/bin/bash
# URP Project Initialization Script
# This script is persistent and stays with your project

# Set project-specific environment variables here
export URP_PROJECT="${URP_PROJECT:-.}"
export URP_SESSION_ID="${URP_SESSION_ID:-unknown}"

echo "URP Environment initialized for: $URP_PROJECT"
