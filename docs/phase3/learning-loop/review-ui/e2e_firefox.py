import sys, time, os
from selenium import webdriver
from selenium.webdriver.firefox.options import Options
from selenium.webdriver.common.by import By
from selenium.webdriver.common.keys import Keys
from selenium.webdriver.common.action_chains import ActionChains
from selenium.webdriver.support.ui import WebDriverWait
base, out = sys.argv[1], sys.argv[2]
os.makedirs(out, exist_ok=True)
o = Options(); o.add_argument("-headless"); o.binary_location = "/Applications/Firefox.app/Contents/MacOS/firefox"
d = webdriver.Firefox(options=o); d.set_window_size(1440, 1000)
W = WebDriverWait(d, 10)
def shot(n): d.save_screenshot(f"{out}/{n}.png"); print("shot", n)
def ok(c, m): print(("PASS " if c else "FAIL ") + m)
def steady():
    # the ux-v2 cards lift on hover and transition; pin the layout so
    # coordinate clicks (and the fixed bulk bar) stay deterministic
    d.execute_script("var s=document.createElement('style');s.textContent='*{transition:none!important;transform:none!important}';document.head.appendChild(s)")
def vis(el): d.execute_script("arguments[0].scrollIntoView({block:'center'})", el); time.sleep(.1)
try:
    d.get(base + "/")
    d.execute_script("localStorage.setItem('ri-theme','light')"); d.get(base + "/"); time.sleep(.5); steady()
    ok(d.find_element(By.TAG_NAME, "html").get_attribute("data-theme") == "light", "light theme applied")
    shot("01-inbox-light-collapsed")
    d.find_element(By.CSS_SELECTOR, "#filterbox summary").click(); time.sleep(.3); shot("01b-filters-open")
    d.find_element(By.CSS_SELECTOR, "#filterbox summary").click()
    cards = d.find_elements(By.CSS_SELECTOR, ".card")
    ok(len(cards) == 10, f"10 pending cards ({len(cards)})")
    # expand first card
    cards[0].find_element(By.CSS_SELECTOR, ".toggle").click()
    W.until(lambda x: cards[0].find_element(By.CSS_SELECTOR, ".detail-inner").get_attribute("data-loaded") == "1")
    ok("open" in cards[0].get_attribute("class"), "card expands on click")
    ok(cards[0].find_element(By.CSS_SELECTOR, ".toggle").get_attribute("aria-expanded") == "true", "aria-expanded true")
    time.sleep(.4); shot("02-card-expanded")
    cards[0].find_element(By.CSS_SELECTOR, ".toggle").click(); time.sleep(.4)
    ok("open" not in cards[0].get_attribute("class"), "card collapses")
    # expand all / collapse all
    d.find_element(By.ID, "expandall").click()
    W.until(lambda x: len(d.find_elements(By.CSS_SELECTOR, ".card.open")) == 10)
    W.until(lambda x: len(d.find_elements(By.CSS_SELECTOR, ".detail-inner[data-loaded='1']")) == 10)
    ok(True, "expand all opens 10")
    d.find_element(By.ID, "collapseall").click(); time.sleep(.3)
    ok(len(d.find_elements(By.CSS_SELECTOR, ".card.open")) == 0, "collapse all")
    # selection
    sels = d.find_elements(By.CSS_SELECTOR, ".sel")
    vis(sels[1]); sels[1].click()
    ok(d.find_element(By.ID, "bulkform").is_displayed(), "bulk bar appears on select")
    vis(sels[4])
    ActionChains(d).key_down(Keys.SHIFT).click(sels[4]).key_up(Keys.SHIFT).perform()
    n = int(d.find_element(By.ID, "bulkcount").text)
    ok(n == 4, f"shift-click range selects 4 ({n})")
    shot("03-bulk-bar")
    d.find_element(By.ID, "selectall").click()
    ok(int(d.find_element(By.ID, "bulkcount").text) == 10, "select all in filter = 10")
    d.find_element(By.ID, "selectall").click()
    ok(int(d.find_elements(By.CSS_SELECTOR, ".sel:checked").__len__()) == 0 and not d.find_element(By.ID, "bulkform").is_displayed(), "deselect all hides bar")
    # keyboard
    d.execute_script("document.activeElement.blur(); window.scrollTo(0,0)")
    ActionChains(d).send_keys("j").perform(); time.sleep(.2)
    ok(d.find_element(By.CSS_SELECTOR, ".card.focus") is not None, "j focuses a card")
    ActionChains(d).send_keys(Keys.ENTER).perform()
    W.until(lambda x: d.find_element(By.CSS_SELECTOR, ".card.focus .detail-inner").get_attribute("data-loaded") == "1")
    ok("open" in d.find_element(By.CSS_SELECTOR, ".card.focus").get_attribute("class"), "Enter expands focused card")
    ActionChains(d).send_keys("x").perform()
    ok(int(d.find_element(By.ID, "bulkcount").text) == 1, "x selects focused card")
    ActionChains(d).send_keys("a").perform(); time.sleep(.2)
    ok(len(d.find_elements(By.CSS_SELECTOR, ".card.focus .btn.armed")) == 1, "a arms Accept")
    ActionChains(d).send_keys("r").perform(); time.sleep(.2)
    ae = d.switch_to.active_element
    ok(ae.tag_name == "textarea", "r arms Reject and focuses the reason")
    shot("04-armed-reject")
    ActionChains(d).send_keys("?").perform()
    ae.send_keys(Keys.ESCAPE)
    # help
    d.find_element(By.ID, "helpbtn").click(); time.sleep(.2)
    ok(d.find_element(By.ID, "help").is_displayed(), "help opens"); shot("05-help")
    d.find_element(By.ID, "help").click()
    # reviewer + bulk accept flow
    d.get(base + "/"); steady()
    r = d.find_element(By.ID, "reviewer"); r.clear(); r.send_keys("dana")
    sels = d.find_elements(By.CSS_SELECTOR, ".sel")
    for s in sels[3:6]: vis(s); s.click()
    d.find_element(By.CSS_SELECTOR, "#bulkform button[value=accept]").click()
    W.until(lambda x: "Confirm bulk accept" in d.page_source)
    ok("3 will be recorded" in d.page_source, "confirmation lists 3 items")
    shot("06-bulk-confirm")
    d.find_element(By.CSS_SELECTOR, "button[name=confirm]").click()
    W.until(lambda x: "Recorded 3 × accept" in d.page_source)
    ok(True, "bulk accept recorded; flash shown"); shot("07-after-bulk")
    # bulk guard with high priority item (a fresh browser session has opened nothing)
    d.delete_all_cookies(); d.get(base + "/"); steady()
    r = d.find_element(By.ID, "reviewer"); r.clear(); r.send_keys("dana")
    d.find_element(By.ID, "selectall").click()
    d.find_element(By.CSS_SELECTOR, "#bulkform button[value=accept]").click()
    W.until(lambda x: "Confirm bulk accept" in d.page_source)
    ok("blocked" in d.page_source, "guard blocks the high-priority/disagreeing items"); shot("08-bulk-blocked")
    # inline reject without reason arms instead of submitting
    d.get(base + "/"); steady()
    c = d.find_elements(By.CSS_SELECTOR, ".card")[-1]
    c.find_element(By.CSS_SELECTOR, ".toggle").click()
    W.until(lambda x: c.find_element(By.CSS_SELECTOR, ".detail-inner").get_attribute("data-loaded") == "1")
    rb = c.find_element(By.CSS_SELECTOR, "button[value=reject]"); vis(rb); rb.click(); time.sleep(.2)
    ok(d.current_url.rstrip("/") == base, "reject without reason does not submit")
    ta = c.find_element(By.CSS_SELECTOR, "form.actions textarea[name=reason]"); vis(ta); ta.send_keys("not what the note says")
    vis(rb); rb.click()
    W.until(lambda x: "Recorded reject" in d.page_source); ok(True, "inline reject with reason recorded"); shot("09-inline-decision")
    # item page + correct form
    d.get(base + "/")
    first = d.find_elements(By.CSS_SELECTOR, ".card .open")[0].get_attribute("href")
    d.get(first); time.sleep(.4); shot("10-item-page")
    d.execute_script("window.scrollTo(0, document.body.scrollHeight)"); time.sleep(.3)
    ActionChains(d).send_keys("c").perform(); time.sleep(.6)
    ok(d.find_element(By.CSS_SELECTOR, "form.correct").is_displayed(), "c opens the correct form"); shot("11-correct-form")
    kind = d.find_elements(By.CSS_SELECTOR, "select.kind")
    if kind:
        from selenium.webdriver.support.ui import Select
        Select(kind[0]).select_by_value("workload-failure")
        ok(d.find_element(By.CSS_SELECTOR, ".class-out").text == "action-required", "class follows kind live")
    # dark
    d.get(base + "/"); steady()
    d.find_element(By.ID, "theme").click(); time.sleep(.3)
    t = d.find_element(By.TAG_NAME, "html").get_attribute("data-theme")
    ok(t == "dark", f"theme toggled light -> {t}")
    d.find_elements(By.CSS_SELECTOR, ".card .toggle")[0].click(); time.sleep(.8)
    shot("12-inbox-dark-expanded")
    d.get(first); time.sleep(.4); shot("13-item-dark")
    d.set_window_size(900, 900); d.get(base + "/"); time.sleep(.4); shot("14-narrow-dark")
finally:
    d.quit()
