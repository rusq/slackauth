(() => {
    if (location.protocol !== "https:" ||
        !(location.hostname === "slack.com" || location.hostname.endsWith(".slack.com")) ||
        !window.navigation) {
        return;
    }
    // Cancel the desktop-app navigation before Chrome's URL policy replaces
    // this interstitial, leaving its browser link available to trapRedirect.
    window.navigation.addEventListener("navigate", (event) => {
        if (event.cancelable && new URL(event.destination.url).protocol === "slack:") {
            event.preventDefault();
        }
    });
})();
