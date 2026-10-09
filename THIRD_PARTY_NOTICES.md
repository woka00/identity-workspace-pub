# Third-party artwork

## Pack 1 — user-provided tracker icon sheet

The 36 user-supplied PNG files in `frontend/public/tracker-icons/` were cropped from the icon sheet supplied by the project owner in the development conversation and bundled locally at 72×72 px. AVATAR.ID does not download these images at runtime.

## Pack 2 — restyled legacy AVATAR.ID tracker icons

The 30 AVATAR.ID PNG files in `frontend/public/tracker-icons/` are restyled raster derivatives of the Font Awesome Free icons that were already used by the previous AVATAR.ID tracker-icon implementation.

- Copyright: Fonticons, Inc.
- Project: https://fontawesome.com/
- Icon license: Creative Commons Attribution 4.0 International (CC BY 4.0)
- Bundled license: `frontend/public/tracker-icons/LICENSE-FONTAWESOME.txt`

The legacy solid silhouettes were normalized to 72×72 transparent PNGs and redrawn as consistent black outline icons to visually match Pack 1. No icon CDN or external runtime request is used.
