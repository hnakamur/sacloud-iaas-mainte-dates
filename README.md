# sacloud-iaas-mainte-dates

これは[さくらのクラウド](https://cloud.sakura.ad.jp/)でホストサーバーがメンテナンス予定になっているIaaSのサーバー情報を取得し、メンテナンス開始日時などとともに一覧表示するコマンドラインツールです。

## インストール方法

Linux amd64の環境であれば以下のコマンドでインストールできます。

```
mkdir -p ~/.local/bin
curl -L https://github.com/hnakamur/sacloud-iaas-mainte-dates/releases/latest/download/sacloud-iaas-mainte-dates.linux_amd64.tar.gz | tar zx -C ~/.local/bin
```

あるいは[The Go Programming Language](https://go.dev/)をインストール済みであれば、以下のコマンドでソースからインストールできます。

```
go install -tags netgo github.com/hankamur/sacloud-iaas-mainte-dates@latest
```

## 利用準備

[Usacloud導入ガイド - Usacloudドキュメント](https://docs.usacloud.jp/usacloud/installation/start_guide/)にしたがって、APIキーの設定を行ってください。

## 使い方

```
$ sacloud-iaas-mainte-dates -h
Usage of sacloud-iaas-mainte-dates:
  -end string
        target maintenance end date in yyyy-mm-dd format
  -format string
        output format (csv, tsv, ltsv, or json) (default "csv")
  -indent
        enable indent for json output
  -profile string
        usacloud profile name
  -start string
        target maintenance start date in yyyy-mm-dd format
  -version
        show version and exit
```
