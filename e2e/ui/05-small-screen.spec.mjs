import {test,expect} from './fixtures.mjs';
import {createWorkflow,script} from '../lib/fixtures.mjs';
import {cleanup} from '../lib/api.mjs';

test.afterEach(cleanup);

for(const [width,lang] of [[320,'zh'],[375,'en']]){
  test(`workbench controls fit a ${width}px ${lang} viewport`,async({page})=>{
    await page.setViewportSize({width,height:780});
    await page.addInitScript(value=>localStorage.setItem('loopforge.lang',value),lang);
    const workflow=await createWorkflow(`small-screen-${width}`,[script('step','true')]);
    await page.goto('/#/agents');
    const search=page.locator('#agentSearch');
    await expect(search).toBeVisible();
    const input=await search.boundingBox();
    expect(input.x+input.width).toBeLessThanOrEqual(width);
    await search.fill('small-screen role');
    await expect(search).toHaveValue('small-screen role');

    await page.goto(`/#/flow/${encodeURIComponent(workflow)}`);
    const toggle=page.locator('#flowPaletteCollapse');
    await expect(toggle).toHaveAttribute('aria-expanded','false');
    const stage=page.locator('#flowStageWrap');
    expect((await stage.boundingBox()).width).toBeGreaterThan(140);
    const node=page.locator('#canvas [data-id="step"]').first();
    await expect.poll(async()=>{
      const [box,canvas]=await Promise.all([node.boundingBox(),stage.boundingBox()]);
      return !!box&&!!canvas&&box.x>=canvas.x&&box.x+box.width<=canvas.x+canvas.width+1;
    }).toBe(true);
    await toggle.click();
    await expect(toggle).toHaveAttribute('aria-expanded','true');
    expect((await stage.boundingBox()).width).toBeGreaterThan(140);
    await toggle.click();
    await page.locator('#canvasZoomIn').click();
    const minimap=page.locator('#canvasMinimap');
    await expect(minimap).toBeHidden();
    await page.locator('#canvasMinimapToggle').click();
    await expect(minimap).toBeVisible();
    await page.setViewportSize({width:1280,height:860});
    await expect(toggle).toHaveAttribute('aria-expanded','true');
  });
}
