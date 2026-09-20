// These signatures were measured from the real Pace firmware's composed 720x576 OSD.
// Every byte outside the six menu rows, the palette, and each complete row must
// agree before naming a menu item. A partial redraw or a new firmware screen is
// deliberately unknown until its pixels have been measured.
const menuPalette = 'eea2702c0ae09542b1348318f896e22700d2d6d7ad73fa7c854b789462740ce2';
const menuOutside = 'aea24907c13449b4f64890c9415f4147a4dcebbca43463970bea3c9f3a45ac28';
const menuRows = [
  { name: 'Movies by Start Time', selected: '2ee0b9b44b50d51ec57206443101d0071564424ba1eb6e6dd3a5a11ee70e9127', plain: '729cfb1de6479a7096b2684861d87e3eec785f51966496c3ed6950f167426abc' },
  { name: 'Movies A–Z', selected: 'f446704f30315119a5b5815be90a36d292a882a80a721fdc4ec95fc20c94bc99', plain: '912ed218b08f9eb8ef21f5ee2019a3b4e1145c049f471e00cfe3c8d26118f841' },
  { name: 'New Movies', selected: 'd48ec90218ff644c2b2632b49cb813c35ac7d699b1cbb65d93b63bc312792285', plain: '52093977d48f2691b09f8e8b751ba545cf460dbd7d43c8af096833892eb3d768' },
  { name: 'Sports & Events', selected: '0dc8b134c43b4d94fb91ef18d4bb8df7c4a72a1346438e03c2c6582eec26b1c0', plain: 'f73e750dad0108099d17af66985f8c4a1142afbb0f00b06c89c948e33de015f8' },
  { name: 'Specialist', selected: '7eefd0a4fc323df953d612205829e59c7ae25a4e8da790f3a9455c358d436451', plain: '3612aa555f96e1ecb5ebfbba4e63a6768c54e68eb4e8adfecb173ba16c43f4df' },
  { name: 'Free Previews', selected: '2b96d6356bcf3cf7475944866684c8310156ef45028b2ab9908755e61b210a44', plain: '0de1e1682f226aa5b6be92da5262ddc3e23693a913c5e1e5f0ea8bed811466f4' },
] as const;

export const unknownScreen = 'Digibox screen changed. A text description of this firmware screen is unavailable.';

async function digest(bytes: Uint8Array): Promise<string> {
  const hash = await crypto.subtle.digest('SHA-256', new Uint8Array(bytes));
  return [...new Uint8Array(hash)].map(byte => byte.toString(16).padStart(2, '0')).join('');
}

export async function describeScreen(pixels: Uint8Array, palette: Uint8Array): Promise<string> {
  if (pixels.length !== 720 * 576 || palette.length !== 256 * 3) return unknownScreen;
  const frame = new Uint8Array(pixels);
  const colours = new Uint8Array(palette);
  const blue = frame[0];
  if (colours[blue * 3] === 0 && colours[blue * 3 + 1] === 5 && colours[blue * 3 + 2] === 69 &&
      frame.every(pixel => pixel === blue)) {
    return 'Plain dark blue Digibox screen. No menu is visible.';
  }

  if (await digest(colours) !== menuPalette) return unknownScreen;
  const outside = new Uint8Array(frame);
  const rowDigests: Promise<string>[] = [];
  for (let row = 0; row < menuRows.length; row++) {
    const bytes = new Uint8Array(480 * 32);
    for (let y = 0; y < 32; y++) {
      const start = (148 + row * 32 + y) * 720 + 120;
      bytes.set(frame.subarray(start, start + 480), y * 480);
      outside.fill(0, start, start + 480);
    }
    rowDigests.push(digest(bytes));
  }
  if (await digest(outside) !== menuOutside) return unknownScreen;
  const rows = await Promise.all(rowDigests);
  let selected = -1;
  for (let row = 0; row < menuRows.length; row++) {
    if (rows[row] === menuRows[row].selected) {
      if (selected !== -1) return unknownScreen;
      selected = row;
    } else if (rows[row] !== menuRows[row].plain) {
      return unknownScreen;
    }
  }
  if (selected === -1) return unknownScreen;
  return `Box Office menu. Six options: ${menuRows.map((row, index) => `${index + 1}, ${row.name}`).join('; ')}. Selected: ${menuRows[selected].name}.`;
}
