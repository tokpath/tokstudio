import {expect,it} from "vitest";
import {previewCollectionFee} from "./collection-fee";
it("adds one fee to external principal without reducing credited funds",()=>{
 expect(previewCollectionFee("100","0","0.02",2)).toEqual({amount:"100",fee:"2",total:"102"});
 expect(previewCollectionFee("40","0","0.02",2)).toEqual({amount:"40",fee:"0.8",total:"40.8"});
 expect(previewCollectionFee("0.25","0","0.02",2)).toEqual({amount:"0.25",fee:"0.01",total:"0.26"});
 expect(previewCollectionFee("25","0","0.02",0)).toEqual({amount:"25",fee:"1",total:"26"});
 expect(previewCollectionFee("0","3","0",2)).toBeUndefined();
});
