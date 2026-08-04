import { renderHostsPage } from "./pages/hostsPage";
import { renderKeysPage } from "./pages/keysPage";
import { renderSettingsPage } from "./pages/settingsPage";
import { renderBackupPage } from "./pages/backupPage";

// Navegação lateral simples: uma seção por domínio, mapeando 1:1 pros
// grupos de métodos de app_*.go.
type PageID = "hosts" | "keys" | "backup" | "settings";

const pages: Record<PageID, (root: HTMLElement) => void> = {
    hosts: renderHostsPage,
    keys: renderKeysPage,
    backup: renderBackupPage,
    settings: renderSettingsPage,
};

const content = document.getElementById("content") as HTMLElement;
const navButtons = document.querySelectorAll<HTMLButtonElement>("[data-page]");

function navigate(page: PageID) {
    for (const btn of navButtons) {
        btn.classList.toggle("active", btn.dataset.page === page);
    }
    pages[page](content);
}

for (const btn of navButtons) {
    const page = btn.dataset.page as PageID | undefined;
    if (!page) continue;
    btn.addEventListener("click", () => navigate(page));
}

navigate("hosts");
