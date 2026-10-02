import os, sys, time
os.environ["MOZ_NO_REMOTE"]="1"
from selenium import webdriver
from selenium.webdriver.firefox.options import Options
from selenium.webdriver.common.by import By
base,out=sys.argv[1],sys.argv[2]
o=Options(); o.add_argument("-headless"); o.binary_location="/Applications/Firefox.app/Contents/MacOS/firefox"
d=webdriver.Firefox(options=o); d.set_window_size(1440,900)
try:
    d.get(base+"/"); d.execute_script("localStorage.setItem('ri-theme','light')")
    d.get(base+"/"); d.find_element(By.XPATH,"//a[contains(@class,'tile')][.//span[text()='High priority']]").click(); time.sleep(.5)
    print(d.current_url, len(d.find_elements(By.CSS_SELECTOR,".card")))
    d.save_screenshot(out+"/30-high-priority-filter-light.png")
    d.execute_script("localStorage.setItem('ri-theme','dark')"); d.get(d.current_url); time.sleep(.4)
    d.save_screenshot(out+"/30-high-priority-filter-dark.png")
finally: d.quit()
