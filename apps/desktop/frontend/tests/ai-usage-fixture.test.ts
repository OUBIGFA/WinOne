import {expect,it} from "vitest";
import {usageFixture} from "./ai-usage-fixture";

it("leaves unrelated null requests to the configuration preview handler",()=>{
 expect(usageFixture("ai.configuration.get","null")).toBeUndefined();
 expect(usageFixture("ai.accounts.list","null")).toBeUndefined();
 expect(usageFixture("ai.usage.get","null")).toMatchObject({offset:0,limit:15});
 expect(usageFixture("ai.accounts.quota","null")).toHaveProperty("quotas");
});
