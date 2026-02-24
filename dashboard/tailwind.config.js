/** @type {import('tailwindcss').Config} */
module.exports = {
    content: [
        "./pages/**/*.{js,ts,jsx,tsx}",
        "./components/**/*.{js,ts,jsx,tsx}",
    ],
    theme: {
        container: {
            center: true,
            padding: "1.5rem",
        },
        extend: {
            fontFamily: {
                sans: ["IBM Plex Sans", "ui-sans-serif", "system-ui"],
                mono: ["IBM Plex Mono", "ui-monospace", "SFMono-Regular"],
            },
            colors: {
                obsidian: {
                    900: '#0b1220',
                    800: '#111827',
                    700: '#1f2937',
                },
                accent: {
                    500: '#f97316',
                    600: '#ea580c',
                }
            }
        },
    },
    plugins: [],
}
