// Vite ?raw imports return the file's text content as a string.
declare module "*.svg?raw" {
  const content: string;
  export default content;
}
