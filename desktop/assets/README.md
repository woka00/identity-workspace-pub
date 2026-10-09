# Desktop icon

`icon.png` is the desktop-specific 512 × 512 RGBA icon used by the window and electron-builder. Its transparent outer margins reduce the visible tile size in the dock. The web/PWA icons remain independent.

Source reference: `frontend/public/identity-workspace-icon-512-v2.png`.
Created with the built-in image_gen tool, then exported at 512 × 512 using Electron nativeImage.

Final image edit prompt:

> Production app icon asset edit, precise-object-edit. Perform only a uniform downscale of the entire supplied image to 84% of a square output canvas and center it with exactly symmetric 8% transparent padding on every side. Original white tile should have perfectly smooth modest rounded corners, no outline. Preserve original black geometric symbol exactly and its relative size within the white tile. Output a crisp clean UI asset with genuine transparent alpha background, absolutely no stray pixels, no noisy edges, no textures, no shadows or outlines, no perspective, no redesign. Canvas square, tile centered both vertically and horizontally. White tile spans x=8% to92%, y=8% to92%. Corner radius 15% of tile width. Black logo shape unmodified. This is a simple clean raster icon export, not artwork.
