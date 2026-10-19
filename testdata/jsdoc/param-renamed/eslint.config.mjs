import jsdoc from "eslint-plugin-jsdoc";

export default [
  {
    files: ["**/*.js"],
    plugins: { jsdoc },
    rules: {
      "jsdoc/check-param-names": "error",
      "jsdoc/require-param": "error",
      "jsdoc/check-types": "error",
      "jsdoc/no-undefined-types": "error",
    },
  },
];
