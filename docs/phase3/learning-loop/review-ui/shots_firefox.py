import os, sys, time, re
os.environ["MOZ_NO_REMOTE"]="1"
from selenium import webdriver
from selenium.webdriver.firefox.options import Options
from selenium.webdriver.common.by import By
base,out=sys.argv[1],sys.argv[2]
os.makedirs(out, exist_ok=True)
o=Options(); o.add_argument("-headless"); o.binary_location="/Applications/Firefox.app/Contents/MacOS/firefox"
d=webdriver.Firefox(options=o); d.set_window_size(1440,1000)
try:
    d.get(base+"/?status=all"); links=[a.get_attribute("href") for a in d.find_elements(By.CSS_SELECTOR,".card .open")]
    for theme in ("light","dark"):
        d.execute_script(f"localStorage.setItem('ri-theme','{theme}')")
        for l in links:
            d.get(l); src=d.page_source
            if "Rendered delta" in src:
                d.execute_script("document.getElementById('rendered').scrollIntoView()"); time.sleep(.3); d.save_screenshot(f"{out}/20-rendered-delta-{theme}.png")
            if "Consensus requests ACTION REQUIRED" in src:
                d.execute_script("document.querySelector('.consensus-action').scrollIntoView()"); time.sleep(.3); d.save_screenshot(f"{out}/23-consensus-action-{theme}.png")
            if "consensus · same-model · 2 calls" in src and "models disagree" not in src:
                d.execute_script("document.getElementById('differ').scrollIntoView()"); time.sleep(.3); d.save_screenshot(f"{out}/24-same-model-consensus-{theme}.png")
            if "same-model consensus · 2 calls" in src:
                d.execute_script("document.getElementById('differ').scrollIntoView()"); time.sleep(.3); d.save_screenshot(f"{out}/21-differ-cells-{theme}.png")
                d.execute_script("document.querySelector('.decide').scrollIntoView()"); time.sleep(.3); d.save_screenshot(f"{out}/22-decide-{theme}.png")
finally: d.quit()
