# PBFT(Practical Byzantine Fault Tolerance) Algorithm

## <b>1. Architecture</b>

<img src="pbft-consensus-behavior.png" alt="drawing" width="700"/><br>

## <b>2. �ý��� ��(��������) ���</b >
* �޽��� ���� ����, ����, ������ ������ �񵿱� ��Ʈ��ũ ȯ�� ���.
* Crash fault�� �޸�, �Ϻη� �߸��� �޽����� ���� �ϰų�  ������ ������ ������Ű�� �������� ���(Byzantine)�� �ִٰ� ����.
* �޽����� �������� �����Ѵ�(liveness)
* ���ĪŰ ��ȣȭ �� ����, �ؽð� ���� ���� �Ϻ��� �ŷڼ��� ������� �ʴ� ȯ�濡���� ���Ἲ �� �۽��� Ȯ���� ������ ������.


## <b>3. �� ���ǿ� �ʿ��� ��ü ����(N)�� 3f + 1�ϱ�?</b>
��ü ��� ���� N�̰�, ��� �Ǵ� �������� ��尡 f�� �϶�, �������� ���Ǹ� ���� �ּ����� ��� ���� N=3f+1�̴�.<br>
PBFT������ 2���� ��� ��Ȳ�� �ִµ� ���� ���������� �޽����� �������� ���۵��� �ʴ� f���� ��찡 �ִ�. �̶� ���Ǹ� ���ؼ��� ��ü N���� ��� ����� f���� �� N - f���� ��忡 ���������� �߸��� �޽����� ������ ��� f�븦 �� (N - f) - f���� ���� ���Ǹ� �̷��� �Ѵ�. ���Ǹ� �̷�� ���ؼ�, (N - f) - f���� ��尡 ��� �Ǵ� �������� ����� f�� ���� ���ƾ� �ϴ� (N - f) - f > f ������ �����Ǳ� ������ N > 3f�� �����ϴ� �ּ����� ���� <b>N = 3f + 1</b>�� ���� �ȴ�.

## <b>4. ���۰���(Normal Case)</b>
### <b>4.1 ���</b>
* client�� ���� ������ request �޽����� primary��忡�� �����Ѵ�.
* client�� ���� request�޽����� ���Ź��� primary���� ������ backup���鿡�� request�޽����� ��� pre-prepare �޽����� �����Ѵ�.(Primary��� �޽��� ���� �ܰ�)
* primary�� ���� pre-prepare�޽����� ���Ź��� backup������ prepare, commit phase�� ���� ���� ������� ���� �Ѵ�.(�޽��� ���� �� ���� �ܰ�)
* client ���� f + 1���� ������ ������ ������ �������� ���̶�� �Ǵ��Ѵ�.
### <b>4.2 �� phase�� ��</b>
#### 4.2.1 <b>Phase1 - Request(<REQUEST,o,t,c>s_c)</b>
* Client�� Primary��忡�� �۾� o, ��û�ð� t, client �ĺ� ID c�� �����Ͽ� ������ �߰��� �� primary��忡�� �����Ѵ�.
```
<REQUEST,o,t,c>s_c

o: client�� ��û�� �۾�
t: ��û �ð�
c: client �ĺ� ID
s_c: client�� ���� (����:<REQUEST,o,t,c>s_c�� <REQUEST,o,t,c>�޽����� client c�� ������ ���� )
```

#### 4.2.2 <b>Phase2 - Pre-prepare(<<PRE-PREPARE, v, n, d>s_p, m>)</b>
* ������ �޽����� �����ϰ� �޽����� v,n�� �ο��ϱ� ���� �ܰ��̴�.
* Client�� ���� request �޽����� ���� Primary���� �ٸ� backup��忡�� Pre-Prepare�޽����� ��Ƽĳ��Ʈ �Ѵ�.
* Pre-prepare�޽����� ���� backup������ ���� n���� �������� ������ �޾Ҵ� Pre-prepared �޽������� ū ���� ���� Ȯ�� �Ѵ�.
* �޽��� �ؽ��� d�� request �޽��� m�� ���Ͽ� Primary��尡 ���� request �޽����� ���۵��� �ʾ����� ���� �� �� �ְ�, Primary����� ������ ���� Pre-prepare�޽����� ���۵��� �ʾ����� Ȯ�� �� �� �ִ�.
* n��° ���ڰ� �� �޽��� ���ۿ��θ� ������ ��, �� backup ������ �ڽ��� ������ �ٸ� backup ���� �� Primary��忡�� Prepare �޽����� ��Ƽ ĳ��Ʈ �Ѵ�.
```
<<PRE-PREPARE, v, n, d>s_p, m>

v: View number(Primary ���ID)
n: �޽��� ���� ��ȣ
d: request�޽����� �ؽ���
s_p: Primary�� ����
```

#### 4.2.3 <b>Phase3 - Prepare(<PREPARE, v, n, d, i>s_i)</b>
* �޽��� ���� �ܰ��̴�.
* �ٸ� backup����� ���� Prepare�޽����� ���� backup���(����)�� �ڽ��� ���� request �޽����� v, n ���� ������ �� �� ��, ���� �޽����� <b>2f</b>���� �Ǹ� ���¸� Prepared�� �ٲ۴�.(�� 2f�� ������ ���� ����)
* Prepared��(�ڽ� ���� �޽����� 2f+1��) backup���� Commit�޽����� �ٸ� backup���鿡�� ��Ƽ ĳ��Ʈ �Ѵ�.
```
<PREPARE, v, n, d, i>s_i

v: View number(Primary ���ID)
n: message sequence number
d: request�޽����� �ؽ���
s_i: i ����� ����
```

#### 4.2.4 <b>Phase4 - Commit(<COMMIT, v, n, d, i>s_i)</b>
* �޽��� ���� �ܰ��̴�.
* Prepared�� ���� ���������� client�� �޽����� ���� �ϱ� ���� �ٸ� ���� ���� Commit�޽����� ������, �� ������ �޽����� <b>2f</b>�� �Ǹ� ���������� ���´� Commited�� �ȴ�.
* Committed�� �� backup���� ���� �� ������� �����Ͽ� Reply�޽���(<REPLY, v, t, c, i, r>s_i)�� Client���� ������.
* Client�� Reply�޽����� <b>f+1</b>�� ������ �ŷ� �� �� �ִ� ����� �����Ѵ�.(�� f+1������ ���� ����)
```
<COMMIT, v, n, d, i>s_i

v: View number(Primary ���ID)
n: message sequence number
d: request�޽����� �ؽ���
s_i: i ����� ����
```

## <b>5. Code structure of the implementation</b>


![](./pbft-consensus-architecture.png)

## <b>6. Working Screenshot</b>
![](./working-screenshot.png)

## License
Apache 2.0