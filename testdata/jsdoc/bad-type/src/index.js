/**
 * Shouts a label.
 * @param {String} label The label.
 * @param {Widget} widget Not defined anywhere.
 * @returns {string} The shout.
 */
export function shout(label, widget) {
  return `${label}${widget}`.toUpperCase();
}
