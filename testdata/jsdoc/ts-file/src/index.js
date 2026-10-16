/**
 * @typedef {object} Point
 * @property {number} x Horizontal position.
 * @property {number} y Vertical position.
 */

/**
 * Moves a point.
 * @param {Point} p The point.
 * @param {number} dx Horizontal delta.
 * @returns {Point} The moved point.
 */
export function move(p, dx) {
  return { x: p.x + dx, y: p.y };
}
