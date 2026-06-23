// Offline Font Awesome icons for <wa-icon>.
//
// WebAwesome's default <wa-icon> library fetches SVGs from the Font Awesome CDN
// at runtime, which breaks our fully-offline/CSP setup. Instead we register a
// custom "fa" library backed by the SVG files bundled from the official
// @fortawesome/fontawesome-free package (imported as raw strings by Vite), so
// every icon ships inside the bundle. Use: <wa-icon library="fa" name="plus">.
import "@awesome.me/webawesome/dist/components/icon/icon.js";
import { registerIconLibrary } from "@awesome.me/webawesome/dist/components/icon/library.js";

import plus from "@fortawesome/fontawesome-free/svgs/solid/plus.svg?raw";
import penToSquare from "@fortawesome/fontawesome-free/svgs/solid/pen-to-square.svg?raw";
import list from "@fortawesome/fontawesome-free/svgs/solid/list.svg?raw";
import tableCells from "@fortawesome/fontawesome-free/svgs/solid/table-cells.svg?raw";
import layerGroup from "@fortawesome/fontawesome-free/svgs/solid/layer-group.svg?raw";
import bullseye from "@fortawesome/fontawesome-free/svgs/solid/bullseye.svg?raw";
import filter from "@fortawesome/fontawesome-free/svgs/solid/filter.svg?raw";
import listCheck from "@fortawesome/fontawesome-free/svgs/solid/list-check.svg?raw";

const ICONS: Record<string, string> = {
  plus,
  "pen-to-square": penToSquare,
  list,
  "table-cells": tableCells,
  "layer-group": layerGroup,
  bullseye,
  filter,
  "list-check": listCheck,
};

registerIconLibrary("fa", {
  // The bundled SVGs have no fill, which would render solid black. Force
  // currentColor so icons inherit the button/text color, then hand the icon
  // element a data: URI (no network).
  resolver: (name: string) => {
    const svg = ICONS[name];
    if (!svg) return "";
    const colored = svg.replace("<svg ", '<svg fill="currentColor" ');
    return `data:image/svg+xml,${encodeURIComponent(colored)}`;
  },
});
