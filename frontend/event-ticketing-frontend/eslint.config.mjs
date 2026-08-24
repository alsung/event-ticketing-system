import coreWebVitals from "eslint-config-next/core-web-vitals";
import typescript from "eslint-config-next/typescript";

// eslint-config-next v16 ships flat configs directly, so the @eslint/eslintrc
// FlatCompat bridge the create-next-app template used is no longer needed --
// it threw "Converting circular structure to JSON" against this version.
const eslintConfig = [
  { ignores: [".next/**", "node_modules/**", "next-env.d.ts"] },
  ...coreWebVitals,
  ...typescript,
];

export default eslintConfig;
