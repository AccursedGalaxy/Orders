#!/bin/bash

# Check if a version bump type is provided
if [ $# -ne 1 ]; then
    echo "Usage: $0 <major|minor|patch>"
    exit 1
fi

# Read current version
VERSION=$(cat version.txt)
MAJOR=$(echo $VERSION | cut -d. -f1)
MINOR=$(echo $VERSION | cut -d. -f2)
PATCH=$(echo $VERSION | cut -d. -f3)

# Bump version according to argument
case $1 in
    major)
        MAJOR=$((MAJOR + 1))
        MINOR=0
        PATCH=0
        ;;
    minor)
        MINOR=$((MINOR + 1))
        PATCH=0
        ;;
    patch)
        PATCH=$((PATCH + 1))
        ;;
    *)
        echo "Invalid version bump type. Use major, minor, or patch."
        exit 1
        ;;
esac

# Create new version string
NEW_VERSION="${MAJOR}.${MINOR}.${PATCH}"

# Update version file
echo $NEW_VERSION > version.txt

# Create git tag
git add version.txt
git commit -m "Bump version to ${NEW_VERSION}"
git tag -a "v${NEW_VERSION}" -m "Release version ${NEW_VERSION}"

echo "Version bumped to ${NEW_VERSION}"
echo "To push the new version, run:"
echo "git push origin main v${NEW_VERSION}" 