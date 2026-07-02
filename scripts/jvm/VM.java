import java.nio.file.*;

// Mirrors the injected smali Lshield/rt/VM; run()/i4() line-by-line, so running
// it on Go-produced bytecode validates that the on-device interpreter algorithm
// is correct (the JVM crypto/int semantics match Android's ART). Used by
// scripts/validate-vm.sh. Input file: line1=wire(hex) line2=bytecode(hex)
// then "a b expected" lines.
public class VM {
  static byte[] hex(String s){ byte[] b=new byte[s.length()/2];
    for(int i=0;i<b.length;i++) b[i]=(byte)Integer.parseInt(s.substring(2*i,2*i+2),16); return b; }

  static int i4(byte[] bc,int pc){
    int v=(bc[pc]<<24);
    v|=((bc[pc+1]&0xff)<<16);
    v|=((bc[pc+2]&0xff)<<8);
    v|=(bc[pc+3]&0xff);
    return v;
  }

  static int run(byte[] bc,int[] args,byte[] w){
    int LOADARG=w[0]&0xff,CONST=w[1]&0xff,MOVE=w[2]&0xff,ADD=w[3]&0xff,SUB=w[4]&0xff,
        MUL=w[5]&0xff,AND=w[6]&0xff,OR=w[7]&0xff,XOR=w[8]&0xff,ADDLIT=w[9]&0xff,
        MULLIT=w[10]&0xff,RET=w[11]&0xff;
    int numRegs=bc[0]&0xff;
    int[] r=new int[numRegs];
    int pc=1;
    while(true){
      int op=bc[pc]&0xff; pc++;
      if(op==LOADARG){int d=bc[pc++]&0xff,ai=bc[pc++]&0xff; r[d]=args[ai]; continue;}
      if(op==CONST){int d=bc[pc++]&0xff; int im=i4(bc,pc); pc+=4; r[d]=im; continue;}
      if(op==MOVE){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; r[d]=r[s]; continue;}
      if(op==ADD){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]+r[b]; continue;}
      if(op==SUB){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]-r[b]; continue;}
      if(op==MUL){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]*r[b]; continue;}
      if(op==AND){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]&r[b]; continue;}
      if(op==OR){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]|r[b]; continue;}
      if(op==XOR){int d=bc[pc++]&0xff,a=bc[pc++]&0xff,b=bc[pc++]&0xff; r[d]=r[a]^r[b]; continue;}
      if(op==ADDLIT){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; int im=i4(bc,pc); pc+=4; r[d]=r[s]+im; continue;}
      if(op==MULLIT){int d=bc[pc++]&0xff,s=bc[pc++]&0xff; int im=i4(bc,pc); pc+=4; r[d]=r[s]*im; continue;}
      if(op==RET){int s=bc[pc++]&0xff; return r[s];}
      return -1;
    }
  }

  public static void main(String[] a) throws Exception {
    var lines=Files.readAllLines(Paths.get(a[0]));
    byte[] w=hex(lines.get(0)), bc=hex(lines.get(1));
    boolean allok=true;
    for(int i=2;i<lines.size();i++){
      var p=lines.get(i).trim().split("\\s+");
      int x=Integer.parseInt(p[0]),y=Integer.parseInt(p[1]),exp=Integer.parseInt(p[2]);
      int got=run(bc,new int[]{x,y},w);
      boolean ok=got==exp; allok&=ok;
      System.out.println("run("+x+","+y+")="+got+" expected="+exp+(ok?" OK":" MISMATCH"));
    }
    System.out.println(allok?"VM-JVM-VALIDATION OK":"VM-JVM-VALIDATION FAILED");
    if(!allok) System.exit(1);
  }
}
